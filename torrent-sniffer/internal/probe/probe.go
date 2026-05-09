package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func hasVideoStream(data []byte, tool string) bool {
	s := string(data)
	if tool == "ffprobe" {
		return (strings.Contains(s, "\"codec_type\": \"video\"") || strings.Contains(s, "\"codec_type\":\"video\"")) && strings.Contains(s, "\"width\":")
	}
	return (strings.Contains(s, "\"@type\": \"Video\"") || strings.Contains(s, "\"@type\":\"Video\"")) && strings.Contains(s, "\"Width\":")
}

// RunFFprobe runs ffprobe on the given file path and returns the JSON output.
func RunFFprobe(ctx context.Context, filePath string) (json.RawMessage, error) {
	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-show_format",
		"-show_streams",
		"-print_format", "json",
		filePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		if stderr.Len() == 0 && stdout.Len() == 0 && strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("ffprobe não instalado no sistema: %w", err)
		}
		log.Printf("[PROBE] ffprobe failed: %v | stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("ffprobe failed: %s: %w", stderr.String(), err)
	}
	log.Printf("[PROBE] ffprobe ran successfully")
	
	return json.RawMessage(stdout.Bytes()), nil
}

// RunMediaInfo runs mediainfo on the given file path and returns the JSON output.
func RunMediaInfo(ctx context.Context, filePath string) (json.RawMessage, error) {
	cmd := exec.CommandContext(ctx,
		"mediainfo",
		"--Output=JSON",
		filePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		if stderr.Len() == 0 && stdout.Len() == 0 && strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("mediainfo não instalado no sistema: %w", err)
		}
		log.Printf("[PROBE] mediainfo failed: %v | stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("mediainfo failed: %s: %w", stderr.String(), err)
	}
	log.Printf("[PROBE] mediainfo ran successfully")
	
	return json.RawMessage(stdout.Bytes()), nil
}

// ProbeFile attempts to use ffprobe first, and if it fails (e.g. incomplete headers),
// falls back to mediainfo. It returns the JSON result and the name of the tool that succeeded.
func ProbeFile(ctx context.Context, filePath string) (json.RawMessage, string, error) {
	// Try mediainfo first (User preference for better bitrate extraction)
	log.Printf("[PROBE] Starting probe attempt 1 (mediainfo) on %s", filePath)
	resMediaInfo, errMediaInfo := RunMediaInfo(ctx, filePath)
	if errMediaInfo == nil {
		if hasVideoStream(resMediaInfo, "mediainfo") {
			return resMediaInfo, "mediainfo", nil
		}
		errMediaInfo = fmt.Errorf("mediainfo succeeded but found no video tracks / IsTruncated (possibly missing moov headers)")
		log.Printf("[PROBE] %v", errMediaInfo)
	}
	
	mediaInfoErr := errMediaInfo.Error()

	// Fallback to ffprobe
	log.Printf("[PROBE] Attempt 1 failed. Starting probe attempt 2 (ffprobe) on %s. (Erro mediainfo: %v)", filePath, errMediaInfo)
	resFFprobe, errFFprobe := RunFFprobe(ctx, filePath)
	if errFFprobe == nil {
		if hasVideoStream(resFFprobe, "ffprobe") {
			return resFFprobe, "ffprobe", nil
		}
		errFFprobe = fmt.Errorf("ffprobe succeeded but found no video streams (possibly missing moov headers)")
		log.Printf("[PROBE] %v", errFFprobe)
	}
	
	ffprobeErr := errFFprobe.Error()

	// Format a friendlier error if executables fail
	friendlyErr := fmt.Sprintf("mediainfo error: %s | ffprobe error: %s", mediaInfoErr, ffprobeErr)
	
	// If both failed, return a combined error
	log.Printf("[PROBE] Both mediainfo and ffprobe probes failed for %s", filePath)
	return nil, "", fmt.Errorf("Erro crítico: As ferramentas de extração de metadados falharam. Verifique se o mediainfo e o FFmpeg estão instalados corretamente. Detalhes: %s", friendlyErr)
}
