package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"rockchip-bot/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type DownloadJob struct {
	FileID    string                       `json:"file_id"`
	FileName  string                       `json:"file_name"`
	Original  string                       `json:"original_name"` // Nome original antes de renomear
	FileSize  int                          `json:"file_size"`
	ChatID    int64                        `json:"chat_id"`  // Para persistência
	MsgID     int                          `json:"msg_id"`   // Para persistência
	Progress  int                          `json:"progress"` // Percentual de 0 a 100
	DestPath  string                       `json:"dest_path"` // Nome amigável (ex: filmes)
	FinalDir  string                       `json:"final_dir"` // Caminho completo no disco
	UpdateMsg func(msgID int, text string) `json:"-"`         // Not persistent
}

type Downloader struct {
	bot       *tgbotapi.BotAPI
	jobs      chan *DownloadJob
	semaphore chan struct{}
	token     string

	active    sync.Map // map[string]*DownloadJob for /status
	pending   sync.Map // map[string]*DownloadJob for queue visibility
	failed    sync.Map // map[string]*DownloadJob for /retry
	failedIDs []string // ordered list of failed IDs
	queueLen  int32
}

func NewDownloader(bot *tgbotapi.BotAPI, token string, limit int) *Downloader {
	d := &Downloader{
		bot:       bot,
		jobs:      make(chan *DownloadJob, 1000),
		semaphore: make(chan struct{}, limit),
		token:     token,
	}

	for i := 0; i < limit; i++ {
		go d.worker()
	}

	return d
}

const jobsFile = "jobs.json"

type SavedState struct {
	Pending []DownloadJob `json:"pending"`
	Active  []DownloadJob `json:"active"`
}

func (d *Downloader) SaveState() {
	var state SavedState
	d.pending.Range(func(_, v interface{}) bool {
		state.Pending = append(state.Pending, *(v.(*DownloadJob)))
		return true
	})
	d.active.Range(func(_, v interface{}) bool {
		state.Active = append(state.Active, *(v.(*DownloadJob)))
		return true
	})

	data, _ := json.Marshal(state)
	os.WriteFile(jobsFile, data, 0644)
}

func (d *Downloader) LoadState(updateMsg func(chatID int64, msgID int, text string)) {
	data, err := os.ReadFile(jobsFile)
	if err != nil {
		return
	}

	var state SavedState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	// Move active jobs back to pending since they were interrupted
	all := append(state.Pending, state.Active...)

	for _, j := range all {
		job := j // copy
		// Restore message functionality
		job.UpdateMsg = func(mID int, text string) {
			updateMsg(job.ChatID, mID, text)
		}
		d.AddJob(job)
	}
}

func (d *Downloader) AddJob(job DownloadJob) {
	// 1. Checagem por FileID (Mesmo arquivo físico no Telegram)
	if _, ok := d.active.Load(job.FileID); ok {
		if job.UpdateMsg != nil {
			job.UpdateMsg(job.MsgID, "⚠ Este arquivo já está sendo baixado.")
		}
		return
	}
	if _, ok := d.pending.Load(job.FileID); ok {
		if job.UpdateMsg != nil {
			job.UpdateMsg(job.MsgID, "⚠ Este arquivo já está na fila de espera.")
		}
		return
	}

	// 2. Checagem por Nome (Evitar o mesmo episódio com nomes diferentes na fila)
	duplicateFound := false
	checkName := func(_, v interface{}) bool {
		j := v.(*DownloadJob)
		if j.FileName == job.FileName || j.Original == job.Original || j.FileName == job.Original || j.Original == job.FileName {
			duplicateFound = true
			return false
		}
		return true
	}
	d.active.Range(checkName)
	if !duplicateFound {
		d.pending.Range(checkName)
	}

	if duplicateFound {
		if job.UpdateMsg != nil {
			job.UpdateMsg(job.MsgID, "⚠ Um arquivo com este nome/episódio já está na fila.")
		}
		return
	}

	// 3. Capturar destino atual (congelar localização)
	active, sub := config.GlobalConfig.GetStatus()
	job.DestPath = active
	if sub != "" {
		job.DestPath = fmt.Sprintf("%s/%s", active, sub)
	}
	job.FinalDir = config.GlobalConfig.GetFinalDir()

	namesToCheck := []string{job.FileName, job.Original}
	for _, n := range namesToCheck {
		if n == "" { continue }
		safe := strings.ReplaceAll(n, "/", "_")
		if _, err := os.Stat(filepath.Join(job.FinalDir, safe)); err == nil {
			if job.UpdateMsg != nil {
				job.UpdateMsg(job.MsgID, fmt.Sprintf("✅ Arquivo já existe no destino (%s): \n%s", n, job.FileName))
			}
			return
		}
	}

	// Create a NEW pointer to avoid stack memory reuse bugs
	newJob := job
	d.pending.Store(job.FileID, &newJob)
	atomic.AddInt32(&d.queueLen, 1)
	d.jobs <- &newJob
	d.SaveState()
}

func (d *Downloader) RetryAll() int {
	var count int
	d.failed.Range(func(key, value interface{}) bool {
		job := value.(*DownloadJob)
		d.failed.Delete(key)
		d.AddJob(*job)
		count++
		return true
	})
	d.failedIDs = nil
	return count
}

type StatusResponse struct {
	Active []DownloadJob `json:"active"`
	Queue  []DownloadJob `json:"queue"`
	Failed []DownloadJob `json:"failed"`
}

func (d *Downloader) GetFullStatus() StatusResponse {
	var res StatusResponse
	res.Active = []DownloadJob{}
	res.Queue = []DownloadJob{}
	res.Failed = []DownloadJob{}

	d.active.Range(func(key, value interface{}) bool {
		res.Active = append(res.Active, *(value.(*DownloadJob)))
		return true
	})

	d.pending.Range(func(key, value interface{}) bool {
		res.Queue = append(res.Queue, *(value.(*DownloadJob)))
		return true
	})

	d.failed.Range(func(key, value interface{}) bool {
		res.Failed = append(res.Failed, *(value.(*DownloadJob)))
		return true
	})

	return res
}

func (d *Downloader) worker() {
	for job := range d.jobs {
		atomic.AddInt32(&d.queueLen, -1)
		d.pending.Delete(job.FileID)

		d.semaphore <- struct{}{} // Acquire slot
		d.active.Store(job.FileID, job)
		d.SaveState()

		err := d.downloadFile(job)

		d.active.Delete(job.FileID)
		<-d.semaphore // Release slot
		d.SaveState()

		if err != nil {
			log.Printf("Failed to download %s: %v", job.FileName, err)
			d.failed.Store(job.FileID, job)
			d.failedIDs = append(d.failedIDs, job.FileID)
			if job.UpdateMsg != nil {
				job.UpdateMsg(job.MsgID, fmt.Sprintf("❌ Erro ao baixar \n%s\n%v", job.FileName, err))
			}
		} else {
			if job.UpdateMsg != nil {
				job.UpdateMsg(job.MsgID, fmt.Sprintf("✅ Download completo!\n📂 Destino: %s", job.DestPath))
			}
		}
	}
}

func (d *Downloader) downloadFile(job *DownloadJob) error {
	// 1. Get File Path from Telegram
	fileConfig := tgbotapi.FileConfig{FileID: job.FileID}
	tgFile, err := d.bot.GetFile(fileConfig)
	if err != nil {
		return fmt.Errorf("getFile error: %v", err)
	}

	if tgFile.FilePath == "" {
		return fmt.Errorf("API doesn't return FilePath. Make sure TELEGRAM_LOCAL=1 is correct.")
	}

	log.Printf("Path context: %s (isAbs: %v)", tgFile.FilePath, filepath.IsAbs(tgFile.FilePath))

	// Create safe filename
	safeName := strings.ReplaceAll(job.FileName, "/", "_")
	if safeName == "" {
		safeName = job.FileID + ".mp4"
	}

	if err := os.MkdirAll(job.FinalDir, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %v", err)
	}
	finalPath := filepath.Join(job.FinalDir, safeName)

	// CASO 1: MODO LOCAL (Caminho absoluto ou detectado como local)
	// Se o Telegram API retornar um caminho absoluto, o arquivo já está no nosso disco (montado via volume)
	if filepath.IsAbs(tgFile.FilePath) {
		// Tradução do caminho do Docker para o Host
		hostSrcPath := strings.Replace(tgFile.FilePath, "/var/lib/telegram-bot-api", "/media/biblioteca/config/telegram-bot-api", 1)

		log.Printf("🚀 MODO LOCAL ATIVADO! Movendo diretamente de %s para %s", hostSrcPath, finalPath)
		job.UpdateMsg(job.MsgID, fmt.Sprintf("✅ Localizando arquivo no servidor...\n%s", job.FileName))

		// Tenta mover (rápido se for mesmo disco) ou copiar
		err = os.Rename(hostSrcPath, finalPath)
		if err != nil {
			log.Printf("Rename failed (cross-device?), copying instead: %v", err)
			err = copyFile(job, hostSrcPath, finalPath)
			if err != nil {
				return fmt.Errorf("failed to copy local file: %v", err)
			}
			os.Remove(hostSrcPath)
		}
		log.Printf("✅ Sucesso (Caminho Local): %s", finalPath)
		job.Progress = 100
		return nil
	}

	// CASO 2: MODO HTTP (Fallback se não for local ou falhar detecção)
	log.Printf("🌐 MODO HTTP: Baixando via interface local...")
	tempPath := filepath.Join(config.IncompletePath, fmt.Sprintf("%s_%s", job.FileID, safeName))

	// Open or create temp file
	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open temp file: %v", err)
	}
	defer out.Close()

	stat, err := out.Stat()
	if err != nil {
		return err
	}
	startPos := stat.Size()

	var isComplete bool
	if startPos > 0 && int(startPos) >= job.FileSize && job.FileSize > 0 {
		log.Printf("File already complete in temp: %s", tempPath)
		isComplete = true
	} else {
		fileUrl := fmt.Sprintf("http://127.0.0.1:8082/file/bot%s/%s", d.token, tgFile.FilePath)
		req, err := http.NewRequest("GET", fileUrl, nil)
		if err != nil {
			return err
		}

		if startPos > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startPos))
			log.Printf("Resuming download %s from %d\n", safeName, startPos)
			job.UpdateMsg(job.MsgID, fmt.Sprintf("⏳ Retomando download...\n%s", job.FileName))
		} else {
			job.UpdateMsg(job.MsgID, fmt.Sprintf("⏳ Iniciando download...\n%s", job.FileName))
		}

		client := &http.Client{Timeout: 0}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("http error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("unexpected http status: %s", resp.Status)
		}

		out.Seek(0, io.SeekEnd)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		buf := make([]byte, 1024*1024)
		var downloaded int64 = startPos
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				written, werr := out.Write(buf[:n])
				if werr != nil {
					return fmt.Errorf("write error: %v", werr)
				}
				downloaded += int64(written)

				// Update progress
				if job.FileSize > 0 {
					job.Progress = int((downloaded * 100) / int64(job.FileSize))
				}
			}

			select {
			case <-ticker.C:
				if job.FileSize > 0 {
					job.UpdateMsg(job.MsgID, fmt.Sprintf("⏳ Baixando: %d%%\n%s", job.Progress, job.FileName))
				}
			default:
			}

			if err == io.EOF {
				isComplete = true
				job.Progress = 100
				break
			}
			if err != nil {
				return fmt.Errorf("read error: %v", err)
			}
		}
	}

	if isComplete {
		out.Close()
		err = os.Rename(tempPath, finalPath)
		if err != nil {
			err = copyFile(job, tempPath, finalPath)
			if err == nil {
				os.Remove(tempPath)
			}
		}
	}
	return nil
}

func copyFile(job *DownloadJob, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 1024*1024)
	var copied int64
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		n, err := in.Read(buf)
		if n > 0 {
			written, werr := out.Write(buf[:n])
			if werr != nil {
				return werr
			}
			copied += int64(written)

			if job.FileSize > 0 {
				job.Progress = int((copied * 100) / int64(job.FileSize))
			}
		}

		select {
		case <-ticker.C:
			job.UpdateMsg(job.MsgID, fmt.Sprintf("⏳ Copiando para HD: %d%%\n%s", job.Progress, job.FileName))
		default:
		}

		if err == io.EOF {
			job.Progress = 100
			break
		}
		if err != nil {
			return err
		}
	}

	return out.Sync()
}
