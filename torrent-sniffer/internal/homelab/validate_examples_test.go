package homelab

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Regression tests matching three real mediainfo CLI dumps (user validation).

func TestExample_Whistle2025_1080pScope_10bitSDR(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "6000.000", "OverallBitRate": "7484000", "FileSize": "5621355142"},
      {"@type": "Video", "Width": "1920", "Height": "800", "FrameRate": "23.976", "BitRate": "6201000",
       "BitDepth": "10", "ColorPrimaries": "BT.709", "transfer_characteristics": "BT.709",
       "Writing_library": "x265 4.1+141+28-0dea3ce03:[Windows][Clang 20.1.3][64 bit] 10bit",
       "Encoding_Settings": "rc=abr / bitrate=6200 / ..."},
      {"@type": "Audio", "Format": "E-AC-3", "BitRate": "640000"},
      {"@type": "Audio", "Format": "E-AC-3", "BitRate": "640000"}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 5621355142)
	if r == nil {
		t.Fatal("nil report")
	}
	if !r.LLMBundle.IsSDR {
		t.Fatal("Whistle must be classified SDR (BT.709)")
	}
	if r.LLMBundle.HasDolbyVision {
		t.Fatal("Whistle must be SDR, no DV")
	}
	wantBPP := 6201000.0 / (1920 * 800 * 23.976)
	if math.Abs(r.BPP-wantBPP) > 0.001 {
		t.Fatalf("BPP want ~%.4f got %.4f", wantBPP, r.BPP)
	}
	if r.Veredito != "RECOMENDADO" {
		t.Fatalf("veredito want RECOMENDADO got %q (bpp=%.3f)", r.Veredito, r.BPP)
	}
	if r.NotaFinal < 9 {
		t.Fatalf("nota want >=9 got %d", r.NotaFinal)
	}
}

func TestExample_Avatar2025_4K_DV_HDR10Plus(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "11820.000", "OverallBitRate": "25700000", "FileSize": "38168104960"},
      {"@type": "Video", "Width": "3840", "Height": "2076", "FrameRate": "24.000", "BitRate": "24400000",
       "BitDepth": "10", "ColorPrimaries": "BT.2020", "transfer_characteristics": "PQ",
       "HDR_Format": "Dolby Vision, Version 1.0, Profile 8.1, dvhe.08.06, BL+RPU, HDR10 compatible / SMPTE ST 2094 App 4, Version HDR10+ Profile A, HDR10+ Profile A compatible"},
      {"@type": "Audio", "Format": "E-AC-3", "BitRate": "640000"},
      {"@type": "Audio", "Format": "E-AC-3 JOC", "BitRate": "768000"}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 38168104960)
	if r == nil {
		t.Fatal("nil report")
	}
	if !r.LLMBundle.HasDolbyVision || !r.LLMBundle.HasHDR10Plus {
		t.Fatalf("DV+H10+ flags: DV=%v H10+=%v", r.LLMBundle.HasDolbyVision, r.LLMBundle.HasHDR10Plus)
	}
	if r.LLMBundle.OverallBitRateMbps < 15 {
		t.Fatalf("overall Mbps unexpected: %v", r.LLMBundle.OverallBitRateMbps)
	}
	if r.Veredito != "RECOMENDADO" {
		t.Fatalf("veredito want RECOMENDADO (overall > 15 Mbps, BPP alto) got %q", r.Veredito)
	}
	// Primary audio = highest bitrate → E-AC-3 JOC 768k, not imbalanced vs 24.4 Mbps video (< 30 rule)
	if r.LLMBundle.PrimaryAudioBitrateKbps < 640 {
		t.Fatalf("primary audio kbps: %v", r.LLMBundle.PrimaryAudioBitrateKbps)
	}
}

func TestExample_ShadowsEdge2025_4K_DV_Reference(t *testing.T) {
	raw := []byte(`{
  "media": {
    "track": [
      {"@type": "General", "Duration": "8520.000", "OverallBitRate": "56200000", "FileSize": "59900000000"},
      {"@type": "Video", "Width": "3840", "Height": "2160", "FrameRate": "23.976", "BitRate": "47700000",
       "BitDepth": "10", "ColorPrimaries": "BT.2020", "transfer_characteristics": "PQ",
       "HDR_Format": "Dolby Vision, Version 1.0, Profile 8.1, dvhe.08.06, BL+RPU, HDR10 compatible / SMPTE ST 2086, Version HDR10, HDR10 compatible"},
      {"@type": "Audio", "Format": "MLP FBA 16-ch", "BitRate": "3844000"},
      {"@type": "Audio", "Format": "E-AC-3 JOC", "BitRate": "1024000"},
      {"@type": "Audio", "Format": "DTS XLL", "BitRate": "3623000"}
    ]
  }
}`)
	r := Analyze(json.RawMessage(raw), "mediainfo", 59900000000)
	if r == nil {
		t.Fatal("nil report")
	}
	if !r.LLMBundle.HasDolbyVision {
		t.Fatal("expected DV")
	}
	if r.Veredito != "RECOMENDADO" {
		t.Fatalf("veredito want RECOMENDADO got %q", r.Veredito)
	}
	c := strings.ToLower(r.LLMBundle.PrimaryAudioCodec)
	if !strings.Contains(c, "mlp") && !strings.Contains(c, "truehd") {
		t.Fatalf("expected lossless primary pick, got codec %q", r.LLMBundle.PrimaryAudioCodec)
	}
}
