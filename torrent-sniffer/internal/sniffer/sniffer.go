package sniffer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"torrent-sniffer/internal/db"
	"torrent-sniffer/internal/domain"
	"torrent-sniffer/internal/homelab"
	"torrent-sniffer/internal/probe"
	"torrent-sniffer/internal/torrent"
)

type SniffTask struct {
	ID           string
	Name         string
	Magnet       string
	Status       string
	DownloadedMB float64
	TotalMB      float64
	CancelFunc   context.CancelFunc `json:"-"`
}

type Service struct {
	engine      *torrent.Engine
	db          *db.Database
	sem         chan struct{} // Concurrency limiter
	activeTasks map[string]*SniffTask
	mu          sync.Mutex
}

func NewService(engine *torrent.Engine, database *db.Database, maxConcurrent int) *Service {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Service{
		engine:      engine,
		db:          database,
		sem:         make(chan struct{}, maxConcurrent),
		activeTasks: make(map[string]*SniffTask),
	}
}

func (s *Service) GetActiveTasks() []SniffTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]SniffTask, 0, len(s.activeTasks))
	for _, t := range s.activeTasks {
		res = append(res, *t)
	}
	return res
}

func (s *Service) CancelTask(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.activeTasks[id]; ok {
		if t.CancelFunc != nil {
			t.CancelFunc()
		}
		// Ao cancelar, removemos da lista de tarefas ativas imediatamente
		delete(s.activeTasks, id)
	}
}

func (s *Service) Process(ctx context.Context, req domain.SniffRequest) (*domain.SniffResult, error) {
	// Register task
	taskID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create a new context for this specific task that can be cancelled independently
	taskCtx, taskCancel := context.WithCancel(context.Background())
	defer taskCancel()

	task := &SniffTask{
		ID:         taskID,
		Name:       "Buscando metadados...",
		Magnet:     req.MagnetURI,
		Status:     "Conectando",
		CancelFunc: taskCancel,
	}
	s.mu.Lock()
	s.activeTasks[taskID] = task
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.activeTasks, taskID)
		s.mu.Unlock()
	}()

	// Removed strict timeout as requested, uses task context
	ctx = taskCtx

	// Concurrency limiter
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	task.Status = "Conectando (Buscando Metadata)"
	start := time.Now()

	// Open torrent
	t, targetFile, maxSize, err := s.engine.OpenTorrent(taskCtx, req.MagnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent: %w", err)
	}

	var torrentSize int64
	for _, f := range t.Files() {
		torrentSize += f.Length()
	}

	defer func() {
		tName := t.Name()
		t.Drop()
		if tName != "" {
			basePath := filepath.Join(os.TempDir(), "torrent-sniffer-data", tName)
			os.RemoveAll(basePath)
			os.RemoveAll(basePath + ".part")
		}
	}()

	task.Name = t.Name()
	task.TotalMB = float64(maxSize) / 1024 / 1024
	fileName := targetFile.DisplayPath()

	// Create temp file
	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "sniff-*.tmp")
	if err != nil {
		return nil, err
	}
	tempFileName := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempFileName)
	}()

	reader := targetFile.NewReader()
	defer reader.Close()

	// Ensure we don't leak the cancellation goroutine
	readerDone := make(chan struct{})
	defer close(readerDone)

	// Listen for context cancellation to interrupt the blocking read
	go func() {
		select {
		case <-taskCtx.Done():
			reader.Close()
		case <-readerDone:
		}
	}()

	// Progressive download limits
	chunkSize := int64(50 * 1024 * 1024)           // 50MB
	maxHeadBytes := int64(float64(maxSize) * 0.15) // up to 15%
	if maxHeadBytes < chunkSize {
		maxHeadBytes = chunkSize
	}
	if maxHeadBytes > 200*1024*1024 {
		maxHeadBytes = 200 * 1024 * 1024
	} // cap at 200MB max ever

	var probeData json.RawMessage
	var toolName string
	var currentDownloaded int64

	// Sequential loop: Download 10MB chunks and probe
	for currentDownloaded < maxHeadBytes && currentDownloaded < maxSize {
		if err := taskCtx.Err(); err != nil {
			return nil, err
		}

		bytesToRead := chunkSize
		if maxSize-currentDownloaded < chunkSize {
			bytesToRead = maxSize - currentDownloaded
		}

		if bytesToRead <= 0 {
			break
		}

		task.Status = fmt.Sprintf("Baixando (%.1f MB de %.1f MB planejados)", float64(currentDownloaded)/1024/1024, float64(maxHeadBytes)/1024/1024)

		var copyErr error
		_, copyErr = io.CopyN(tempFile, reader, bytesToRead)
		if copyErr != nil && copyErr != io.EOF {
			return nil, fmt.Errorf("read err at %d: %w", currentDownloaded, copyErr)
		}

		tempFile.Sync()
		currentDownloaded += bytesToRead
		task.DownloadedMB = float64(currentDownloaded) / 1024 / 1024

		task.Status = "Analisando..."
		probeData, toolName, err = probe.ProbeFile(taskCtx, tempFileName)
		if err == nil {
			log.Printf("[SNIFFER] Valid metadata found at %d bytes", currentDownloaded)
			break // found!
		}
		log.Printf("[SNIFFER] Metadata not found in first %d bytes. Expanding...", currentDownloaded)
	}

	// Tail fallback (if still err)
	if err != nil {
		task.Status = "Baixando Fim do Arquivo (100MB Tail)"
		tailSize := int64(100 * 1024 * 1024) // 100MB do final
		if maxSize > currentDownloaded+tailSize {
			_, errSeek := reader.Seek(-tailSize, io.SeekEnd)
			if errSeek == nil {
				// write padding zeros
				tempFile.Seek(currentDownloaded+(maxSize-currentDownloaded-tailSize), io.SeekStart)
				io.Copy(tempFile, reader)
				tempFile.Sync()

				task.Status = "Analisando (Tail)..."
				probeData, toolName, err = probe.ProbeFile(taskCtx, tempFileName)
				if err != nil {
					log.Printf("[SNIFFER] Tail probe err: %v", err)
				} else {
					log.Printf("[SNIFFER] Valid metadata found at TAIL")
				}
			} else {
				log.Printf("[SNIFFER] reader.Seek failed for tail: %v", errSeek)
			}
		} else {
			log.Printf("[SNIFFER] maxSize (%d) not large enough for tail (%d + %d)", maxSize, currentDownloaded, tailSize)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("probing failed after all tries: %w", err)
	}

	fetchedFromTail := task.Status == "Analisando (Tail)..." || strings.Contains(task.Status, "Fim")

	stats := t.Stats()
	seeds := stats.ConnectedSeeders
	peers := stats.ActivePeers

	if s.db != nil {
		s.db.SaveSniff(task.Name, fileName, maxSize, torrentSize, req.MagnetURI, toolName, string(probeData), currentDownloaded, fetchedFromTail, seeds, peers)
	}

	out := &domain.SniffResult{
		TorrentName:     task.Name,
		FileName:        fileName,
		FileSize:        maxSize,
		TorrentSize:     torrentSize,
		Probe:           probeData,
		ProbeTool:       toolName,
		ElapsedMs:       time.Since(start).Milliseconds(),
		DownloadedBytes: currentDownloaded,
		FetchedFromTail: fetchedFromTail,
		Seeds:           seeds,
		Peers:           peers,
	}
	if hr := homelab.Analyze(probeData, toolName, maxSize); hr != nil {
		out.Homelab = hr
	}
	return out, nil
}

func (s *Service) GetHistory(limit int) ([]map[string]interface{}, error) {
	if s.db == nil {
		return []map[string]interface{}{}, nil
	}
	history, err := s.db.GetHistory(limit)
	if err != nil {
		return nil, err
	}
	for _, row := range history {
		probeStr, _ := row["probe"].(string)
		tool, _ := row["probe_tool"].(string)
		var fs int64
		switch v := row["file_size"].(type) {
		case int64:
			fs = v
		case float64:
			fs = int64(v)
		case int:
			fs = int64(v)
		}
		var raw json.RawMessage
		if json.Unmarshal([]byte(probeStr), &raw) == nil {
			if hr := homelab.Analyze(raw, tool, fs); hr != nil {
				row["homelab"] = hr
			}
		}
	}
	return history, nil
}

func (s *Service) DeleteHistory(id string) error {
	if s.db == nil {
		return nil
	}
	return s.db.DeleteSniff(id)
}

func (s *Service) UpdateNotes(id string, notes string) error {
	if s.db == nil {
		return nil
	}
	return s.db.UpdateNotes(id, notes)
}

func (s *Service) FlushHistory() error {
	if s.db == nil {
		return nil
	}
	return s.db.FlushSniffs()
}
