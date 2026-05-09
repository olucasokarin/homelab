package homelab

import (
	"encoding/json"
	"testing"
)

func TestAnalyzeMediaInfo_4K_DV_LowOverall(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "7200.000", "OverallBitRate": "12000000", "FileSize": "10800000000"},
      {"@type": "Video", "Width": "3840", "Height": "2160", "FrameRate": "23.976",
       "BitRate": "10000000", "BitDepth": "10",
       "HDR_Format": "Dolby Vision / HDR10", "HDR_Format_Compatibility": "HDR10",
       "ColorPrimaries": "BT.2020", "Writing_library": "x265 - 3.5+1-f0c4092:[info]: cpuid=0 crf=24.000000",
       "Encoding_Settings": "preset=slower / ..."},
      {"@type": "Audio", "Format": "E-AC-3", "BitRate": "256000"}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 10800000000)
	if r == nil {
		t.Fatal("nil report")
	}
	if r.Veredito != "REJEITADO" {
		t.Fatalf("veredito want REJEITADO got %q", r.Veredito)
	}
	if r.BPP <= 0 {
		t.Fatalf("bpp should be positive, got %v", r.BPP)
	}
	if r.LLMBundle.CRFFromWritingLibrary == nil || *r.LLMBundle.CRFFromWritingLibrary != 24 {
		t.Fatalf("crf parse: %+v", r.LLMBundle.CRFFromWritingLibrary)
	}
}

func TestAnalyzeMediaInfo_10bitSDR(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "5000.000", "OverallBitRate": "25000000"},
      {"@type": "Video", "Width": "1920", "Height": "1080", "FrameRate": "24.000",
       "BitRate": "24000000", "BitDepth": "10", "ColorPrimaries": "BT.709"}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 0)
	if r == nil {
		t.Fatal("nil report")
	}
	if !r.LLMBundle.IsSDR {
		t.Fatalf("expected SDR")
	}
	if r.Veredito != "RECOMENDADO" {
		t.Fatalf("veredito want RECOMENDADO got %q (bpp=%v)", r.Veredito, r.BPP)
	}
}

func TestAnalyzeMediaInfo_UltrafastReject(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "100.000", "OverallBitRate": "80000000"},
      {"@type": "Video", "Width": "3840", "Height": "2160", "FrameRate": "24.000",
       "BitRate": "75000000", "Encoding_Settings": "cpuid=0 / preset=ultrafast / ..."}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 0)
	if r == nil {
		t.Fatal("nil report")
	}
	if r.Veredito != "REJEITADO" {
		t.Fatalf("veredito want REJEITADO for ultrafast, got %q", r.Veredito)
	}
}
