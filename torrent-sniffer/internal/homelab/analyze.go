package homelab

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Report is the Home Lab encoding verdict plus an LLM-oriented field bundle.
type Report struct {
	Veredito       string   `json:"veredito"`
	AnaliseTecnica string   `json:"analise_tecnica"`
	PontosAtencao  []string `json:"pontos_atencao"`
	NotaFinal      int      `json:"nota_final"`

	BPP              float64 `json:"bpp"`
	BitrateVideoBps float64 `json:"bitrate_video_bps"`
	BitrateVideoMbps float64 `json:"bitrate_video_mbps"`
	OverallBitrateMbps float64 `json:"overall_bit_rate_mbps,omitempty"`

	LLMBundle LLMBundle `json:"llm_bundle"`
}

// LLMBundle holds MediaInfo/ffprobe fields that expose weak encodes for LLM review.
type LLMBundle struct {
	WritingLibrary          string   `json:"writing_library,omitempty"`
	EncodingSettings        string   `json:"encoding_settings,omitempty"`
	ColorPrimaries          string   `json:"color_primaries,omitempty"`
	TransferCharacteristics string   `json:"transfer_characteristics,omitempty"`
	HDRFormat               string   `json:"hdr_format,omitempty"`
	HDRFormatCompatibility  string   `json:"hdr_format_compatibility,omitempty"`
	BitDepth                string   `json:"bit_depth,omitempty"`
	Width                   int      `json:"width,omitempty"`
	Height                  int      `json:"height,omitempty"`
	FrameRate               float64  `json:"frame_rate,omitempty"`
	OverallBitRateMbps      float64  `json:"overall_bit_rate_mbps,omitempty"`
	VideoBitRateMbps        float64  `json:"video_bit_rate_mbps,omitempty"`
	CRFFromWritingLibrary   *float64 `json:"crf_from_writing_library,omitempty"`
	PresetHint              string   `json:"preset_hint,omitempty"`
	PrimaryAudioCodec       string   `json:"primary_audio_codec,omitempty"`
	PrimaryAudioBitrateKbps float64  `json:"primary_audio_bitrate_kbps,omitempty"`
	Is2160p                 bool     `json:"is_2160p,omitempty"`
	IsSDR                   bool     `json:"is_sdr,omitempty"`
	HasDolbyVision          bool     `json:"has_dolby_vision,omitempty"`
	HasHDR10Plus            bool     `json:"has_hdr10plus,omitempty"`
	BPPTierNote             string   `json:"bpp_tier_note,omitempty"`
	ProbeTool               string   `json:"probe_tool,omitempty"`
}

var (
	reCRF    = regexp.MustCompile(`(?i)crf[=:\s]+([\d.]+)`)
	rePreset = regexp.MustCompile(`(?i)preset[=:\s]+([a-z0-9_-]+)`)
)

// Analyze builds a Report from raw probe JSON (mediainfo or ffprobe) and torrent file size (bytes).
func Analyze(raw json.RawMessage, tool string, fileSizeBytes int64) *Report {
	tool = strings.ToLower(strings.TrimSpace(tool))
	if len(raw) == 0 {
		return nil
	}
	switch tool {
	case "mediainfo":
		return analyzeMediaInfo(raw, fileSizeBytes)
	case "ffprobe":
		return analyzeFFprobe(raw, fileSizeBytes)
	default:
		return nil
	}
}

func analyzeMediaInfo(raw json.RawMessage, fileSize int64) *Report {
	var root struct {
		Media struct {
			Track []map[string]interface{} `json:"track"`
		} `json:"media"`
	}
	if err := json.Unmarshal(raw, &root); err != nil || len(root.Media.Track) == 0 {
		return nil
	}

	var general, video map[string]interface{}
	var audios []map[string]interface{}
	for _, t := range root.Media.Track {
		typ, _ := t["@type"].(string)
		switch typ {
		case "General":
			general = t
		case "Video":
			if video == nil {
				video = t
			}
		case "Audio":
			audios = append(audios, t)
		}
	}
	if video == nil {
		return nil
	}

	durationSec := parseFloatVal(firstStr(general, "Duration"))
	overallBps := parseFloatVal(firstStr(general, "OverallBitRate"))
	streamVideoBps := parseFloatVal(firstStr(video, "BitRate"))
	if streamVideoBps < 1000 && overallBps > 0 {
		streamVideoBps = overallBps
	}
	var calcBps float64
	if fileSize > 0 && durationSec > 0 {
		calcBps = (float64(fileSize) * 8) / durationSec
	}
	// BPP alinha com MediaInfo "Bits/(Pixel*Frame)": bitrate da faixa de vídeo quando credível.
	// fileSize*8/duração inclui áudio/legendas e infla BPP em MKVs completos (ex.: Whistle).
	videoBps := pickVideoBpsForPicture(streamVideoBps, calcBps, overallBps)

	w := int(parseFloatVal(firstStr(video, "Width")))
	h := int(parseFloatVal(firstStr(video, "Height")))
	fps := parseMediaInfoFrameRate(video)
	hdrFmt := joinHDR(video)
	hdrCompat := strField(video, "HDR_Format_Compatibility")
	prim := strField(video, "Color_primaries")
	if prim == "" {
		prim = strField(video, "ColorPrimaries")
	}
	transfer := strField(video, "transfer_characteristics")
	if transfer == "" {
		transfer = strField(video, "colour_transfer")
	}
	bitDepth := firstStr(video, "BitDepth")

	writing := strField(video, "Writing_library")
	if writing == "" {
		writing = strField(video, "Encoded_Library")
	}
	encSettings := strField(video, "Encoding_Settings")

	hdrLower := strings.ToLower(hdrFmt + " " + hdrCompat)
	hasDV := strings.Contains(hdrLower, "dolby vision") || strings.Contains(hdrLower, "dvhe.") || strings.Contains(hdrLower, "dvh1.")
	hasH10P := strings.Contains(hdrLower, "hdr10+") || strings.Contains(hdrLower, "hdr10 plus") || strings.Contains(hdrLower, "st 2094")
	pqish := strings.Contains(strings.ToLower(transfer), "smpte2084") || strings.Contains(strings.ToLower(transfer), "pq")
	isHDRMeta := hasDV || hasH10P ||
		strings.Contains(hdrLower, "smpte st 2086") || strings.Contains(hdrLower, "hdr10") ||
		(pqish && strings.Contains(strings.ToLower(prim), "2020"))
	isSDR := !isHDRMeta

	presetHint := extractPreset(encSettings)
	crfPtr := extractCRF(writing)

	denom := float64(w) * float64(h) * fps
	var bpp float64
	if denom > 0 && videoBps > 0 {
		bpp = videoBps / denom
	}

	overallMbps := overallBps / 1e6
	if overallMbps <= 0 && fileSize > 0 && durationSec > 0 {
		overallMbps = (float64(fileSize) * 8 / durationSec) / 1e6
	}
	videoMbps := videoBps / 1e6

	is2160 := h >= 2160 || w >= 3840
	is4KHigh := is2160 && videoMbps >= 30

	audioCodec, audioKbps := pickPrimaryAudioMediaInfo(audios)

	bppNote := tierBPP(bpp)
	imbalancedAudio := is4KHigh && isLossyDolbyDigital(audioCodec) && audioKbps > 0 && audioKbps < 640

	rejectDVBitrate := is2160 && (hasDV || hasH10P) && overallMbps > 0 && overallMbps < 15
	lowBPP := bpp > 0 && ((is2160 && bpp < 0.030) || (!is2160 && bpp < 0.025))
	badPreset := presetIsVeryFast(presetHint)
	badCRF := crfPtr != nil && *crfPtr > 22
	tenBitSDR := parseFloatVal(bitDepth) >= 10 && isSDR

	pontos := []string{}
	if rejectDVBitrate {
		pontos = append(pontos, "Bitrate insuficiente para DV/HDR10+ em 2160p (Overall < 15 Mbps).")
	}
	if lowBPP {
		pontos = append(pontos, "BPP baixo (< 0.030): risco de imagem lavada ou blocos em 4K.")
	}
	if bpp >= 0.030 && bpp < 0.050 && !isSDR {
		pontos = append(pontos, "BPP 0.030–0.050: aceitável em SDR; arriscado para HDR.")
	}
	if badPreset {
		pontos = append(pontos, "Preset ultrafast/superfast: artefatos prováveis mesmo com bitrate alto.")
	}
	if badCRF {
		pontos = append(pontos, "CRF > 22 no x265/x264 (writing library): qualidade sacrificada por tamanho.")
	}
	if imbalancedAudio {
		pontos = append(pontos, "Release desequilibrado: vídeo 4K alto bitrate com AC-3/E-AC-3 < 640 kbps.")
	}
	if strings.Contains(strings.ToLower(prim), "2020") && isSDR {
		pontos = append(pontos, "Color primaries BT.2020 em metadados — confirmar se é HDR real ou metadados inconsistentes.")
	}

	veredito := "APENAS SE NÃO HOUVER OUTRO"
	nota := 5

	if bpp == 0 && tenBitSDR {
		nota = 6
	}

	if rejectDVBitrate || lowBPP {
		veredito = "REJEITADO"
		nota = 2
	} else if badPreset || (badCRF && bpp < 0.045) {
		veredito = "REJEITADO"
		nota = 3
	} else if bpp >= 0.060 {
		veredito = "RECOMENDADO"
		nota = 9
		if tenBitSDR {
			nota = 10
		}
	} else if bpp >= 0.040 {
		veredito = "RECOMENDADO"
		nota = 8
	} else if bpp >= 0.025 {
		veredito = "APENAS SE NÃO HOUVER OUTRO"
		nota = 6
		if tenBitSDR {
			nota = 7
		}
	}

	if imbalancedAudio && veredito == "RECOMENDADO" {
		veredito = "APENAS SE NÃO HOUVER OUTRO"
		if nota > 7 {
			nota = 7
		}
	}
	if tenBitSDR && nota < 8 && veredito != "REJEITADO" {
		pontos = append([]string{"10-bit SDR: ideal para reduzir banding sem overhead de HDR."}, pontos...)
	}

	nota = clampInt(nota, 1, 10)

	tech := summarizeTechLine(videoMbps, w, h, fps, bpp)

	bundle := LLMBundle{
		WritingLibrary:          writing,
		EncodingSettings:        encSettings,
		ColorPrimaries:          prim,
		TransferCharacteristics: transfer,
		HDRFormat:               strField(video, "HDR_Format"),
		HDRFormatCompatibility:  hdrCompat,
		BitDepth:                bitDepth,
		Width:                   w,
		Height:                  h,
		FrameRate:               fps,
		OverallBitRateMbps:      overallMbps,
		VideoBitRateMbps:        videoMbps,
		CRFFromWritingLibrary:   crfPtr,
		PresetHint:              presetHint,
		PrimaryAudioCodec:       audioCodec,
		PrimaryAudioBitrateKbps: audioKbps,
		Is2160p:                 is2160,
		IsSDR:                   isSDR,
		HasDolbyVision:          hasDV,
		HasHDR10Plus:            hasH10P,
		BPPTierNote:             bppNote,
		ProbeTool:               "mediainfo",
	}

	return &Report{
		Veredito:           veredito,
		AnaliseTecnica:     tech,
		PontosAtencao:      dedupeStrings(pontos),
		NotaFinal:          nota,
		BPP:                bpp,
		BitrateVideoBps:    videoBps,
		BitrateVideoMbps:   videoMbps,
		OverallBitrateMbps: overallMbps,
		LLMBundle:          bundle,
	}
}

func analyzeFFprobe(raw json.RawMessage, fileSize int64) *Report {
	var root struct {
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
		Streams []map[string]interface{} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	var vmap map[string]interface{}
	var audios []map[string]interface{}
	for _, s := range root.Streams {
		ct, _ := s["codec_type"].(string)
		if ct == "video" && vmap == nil {
			vmap = s
		} else if ct == "audio" {
			audios = append(audios, s)
		}
	}
	if vmap == nil {
		return nil
	}

	durationSec := parseFloatVal(root.Format.Duration)
	overallBps := parseFloatVal(root.Format.BitRate)
	streamVideoBps := parseFloatVal(strField(vmap, "bit_rate"))
	if streamVideoBps < 1000 {
		streamVideoBps = overallBps
	}
	var calcBps float64
	if fileSize > 0 && durationSec > 0 {
		calcBps = (float64(fileSize) * 8) / durationSec
	}
	videoBps := pickVideoBpsForPicture(streamVideoBps, calcBps, overallBps)

	w := int(parseFloatVal(strField(vmap, "width")))
	h := int(parseFloatVal(strField(vmap, "height")))
	fps := parseFFprobeFrameRate(strField(vmap, "r_frame_rate"))
	bitDepth := strField(vmap, "bits_per_raw_sample")
	if bitDepth == "" {
		bitDepth = strField(vmap, "bits_per_sample")
	}
	prim := strField(vmap, "color_primaries")
	transfer := strField(vmap, "color_transfer")
	hdrFmt := strField(vmap, "hdr_format")

	hdrLower := strings.ToLower(hdrFmt + " " + transfer + " " + prim)
	hasDV := strings.Contains(hdrLower, "dolby vision") || strings.Contains(hdrLower, "dvhe.")
	hasH10P := strings.Contains(hdrLower, "hdr10+") || strings.Contains(hdrLower, "plus")
	isHDRMeta := hasDV || hasH10P || strings.Contains(hdrLower, "smpte2084") || strings.Contains(hdrLower, "pq")
	isSDR := !isHDRMeta

	denom := float64(w) * float64(h) * fps
	var bpp float64
	if denom > 0 && videoBps > 0 {
		bpp = videoBps / denom
	}
	overallMbps := overallBps / 1e6
	if overallMbps <= 0 && fileSize > 0 && durationSec > 0 {
		overallMbps = (float64(fileSize) * 8 / durationSec) / 1e6
	}
	videoMbps := videoBps / 1e6

	is2160 := h >= 2160 || w >= 3840
	is4KHigh := is2160 && videoMbps >= 30

	audioCodec, audioKbps := pickPrimaryAudioFFprobe(audios)
	bppNote := tierBPP(bpp)
	imbalancedAudio := is4KHigh && isLossyDolbyDigital(audioCodec) && audioKbps > 0 && audioKbps < 640

	rejectDVBitrate := is2160 && (hasDV || hasH10P) && overallMbps > 0 && overallMbps < 15
	lowBPP := bpp > 0 && ((is2160 && bpp < 0.030) || (!is2160 && bpp < 0.025))
	tenBitSDR := parseFloatVal(bitDepth) >= 10 && isSDR

	pontos := []string{}
	if rejectDVBitrate {
		pontos = append(pontos, "Bitrate insuficiente para DV/HDR10+ em 2160p (Overall < 15 Mbps).")
	}
	if lowBPP {
		pontos = append(pontos, "BPP baixo (< 0.030): risco de imagem lavada ou blocos em 4K.")
	}
	if bpp >= 0.030 && bpp < 0.050 && !isSDR {
		pontos = append(pontos, "BPP 0.030–0.050: aceitável em SDR; arriscado para HDR.")
	}
	if imbalancedAudio {
		pontos = append(pontos, "Release desequilibrado: vídeo 4K alto bitrate com AC-3/E-AC-3 < 640 kbps.")
	}
	pontos = append(pontos, "Metadados via ffprobe: Writing library / Encoding settings podem estar ausentes — preferir MediaInfo quando possível.")

	veredito := "APENAS SE NÃO HOUVER OUTRO"
	nota := 5

	if bpp == 0 && tenBitSDR {
		nota = 6
	}

	if rejectDVBitrate || lowBPP {
		veredito = "REJEITADO"
		nota = 3
	} else if bpp >= 0.060 {
		veredito = "RECOMENDADO"
		nota = 9
		if tenBitSDR {
			nota = 10
		}
	} else if bpp >= 0.040 {
		veredito = "RECOMENDADO"
		nota = 8
	} else if bpp >= 0.025 {
		veredito = "APENAS SE NÃO HOUVER OUTRO"
		nota = 6
		if tenBitSDR {
			nota = 7
		}
	}
	if imbalancedAudio && veredito == "RECOMENDADO" {
		veredito = "APENAS SE NÃO HOUVER OUTRO"
		if nota > 7 {
			nota = 7
		}
	}
	if tenBitSDR && nota < 8 && veredito != "REJEITADO" {
		pontos = append([]string{"10-bit SDR: ideal para reduzir banding sem overhead de HDR."}, pontos...)
	}
	nota = clampInt(nota, 1, 10)

	bundle := LLMBundle{
		ColorPrimaries:          prim,
		TransferCharacteristics: transfer,
		HDRFormat:               hdrFmt,
		BitDepth:                bitDepth,
		Width:                   w,
		Height:                  h,
		FrameRate:               fps,
		OverallBitRateMbps:      overallMbps,
		VideoBitRateMbps:        videoMbps,
		PrimaryAudioCodec:       audioCodec,
		PrimaryAudioBitrateKbps: audioKbps,
		Is2160p:                 is2160,
		IsSDR:                   isSDR,
		HasDolbyVision:          hasDV,
		HasHDR10Plus:            hasH10P,
		BPPTierNote:             bppNote,
		ProbeTool:               "ffprobe",
	}

	return &Report{
		Veredito:           veredito,
		AnaliseTecnica:     summarizeTechLine(videoMbps, w, h, fps, bpp),
		PontosAtencao:      dedupeStrings(pontos),
		NotaFinal:          nota,
		BPP:                bpp,
		BitrateVideoBps:    videoBps,
		BitrateVideoMbps:   videoMbps,
		OverallBitrateMbps: overallMbps,
		LLMBundle:          bundle,
	}
}

func summarizeTechLine(videoMbps float64, w, h int, fps, bpp float64) string {
	if w <= 0 || h <= 0 {
		return "Resolução ou metadados incompletos para relação bitrate/resolução."
	}
	var b strings.Builder
	b.WriteString(strconv.FormatFloat(videoMbps, 'f', 1, 64))
	b.WriteString(" Mbps para ")
	b.WriteString(strconv.Itoa(w))
	b.WriteString("×")
	b.WriteString(strconv.Itoa(h))
	if fps > 0 {
		b.WriteString(" @ ")
		b.WriteString(strconv.FormatFloat(fps, 'f', 3, 64))
		b.WriteString(" fps")
	}
	if bpp > 0 {
		b.WriteString(" · BPP≈")
		b.WriteString(strconv.FormatFloat(bpp, 'f', 4, 64))
	}
	b.WriteString(".")
	return b.String()
}

func firstStr(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			s := stringifyJSON(v)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func strField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	return stringifyJSON(m[key])
}

func stringifyJSON(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return string(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func parseFloatVal(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	// strip non-numeric prefix/suffix like "24.000 (24/1)"
	for i, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			s = s[i:]
			break
		}
	}
	end := 0
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '/' {
			end++
		} else {
			break
		}
	}
	s = s[:end]
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) == 2 {
			a, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			b, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if e1 == nil && e2 == nil && b != 0 {
				return a / b
			}
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseMediaInfoFrameRate(video map[string]interface{}) float64 {
	if video == nil {
		return 0
	}
	num := parseFloatVal(strField(video, "FrameRate_Num"))
	den := parseFloatVal(strField(video, "FrameRate_Den"))
	if num > 0 && den > 0 {
		return num / den
	}
	return parseFloatVal(strField(video, "FrameRate"))
}

func parseFFprobeFrameRate(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) == 2 {
			a, e1 := strconv.ParseFloat(parts[0], 64)
			b, e2 := strconv.ParseFloat(parts[1], 64)
			if e1 == nil && e2 == nil && b != 0 {
				return a / b
			}
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func joinHDR(video map[string]interface{}) string {
	if video == nil {
		return ""
	}
	parts := []string{
		strField(video, "HDR_Format"),
		strField(video, "HDR_Format_String"),
		strField(video, "HDR_Format_Profile"),
	}
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(p)
		}
	}
	return b.String()
}

func extractCRF(writing string) *float64 {
	if writing == "" {
		return nil
	}
	m := reCRF.FindStringSubmatch(writing)
	if len(m) < 2 {
		return nil
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &f
}

func extractPreset(enc string) string {
	if enc == "" {
		return ""
	}
	m := rePreset.FindStringSubmatch(enc)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(m[1])
}

func presetIsVeryFast(p string) bool {
	p = strings.ToLower(p)
	return p == "ultrafast" || p == "superfast"
}

func tierBPP(bpp float64) string {
	if bpp <= 0 {
		return "BPP indisponível (fps/bitrate/resolução incompletos)."
	}
	if bpp < 0.030 {
		return "BPP < 0.030: não recomendado (lavado/blocos)."
	}
	if bpp < 0.050 {
		return "BPP 0.030–0.050: aceitável para SDR; arriscado para HDR."
	}
	if bpp < 0.060 {
		return "BPP 0.050–0.060: bom; ainda abaixo do patamar excelente."
	}
	return "BPP > 0.060: qualidade excelente (transparência relativa)."
}

func isLossyDolbyDigital(codec string) bool {
	c := strings.ToLower(codec)
	return strings.Contains(c, "ac-3") || strings.Contains(c, "e-ac-3") || strings.Contains(c, "ac3") ||
		strings.Contains(c, "eac3") || strings.Contains(c, "dolby digital")
}

func pickPrimaryAudioMediaInfo(tracks []map[string]interface{}) (codec string, kbps float64) {
	var best map[string]interface{}
	var bestBps float64
	for _, t := range tracks {
		bps := parseFloatVal(firstStr(t, "BitRate"))
		if bps > bestBps {
			bestBps = bps
			best = t
		}
	}
	if best == nil && len(tracks) > 0 {
		best = tracks[0]
		bestBps = parseFloatVal(firstStr(best, "BitRate"))
	}
	if best == nil {
		return "", 0
	}
	codec = firstStr(best, "Format", "Format_Commercial_IfAny")
	return codec, bestBps / 1000
}

func pickPrimaryAudioFFprobe(streams []map[string]interface{}) (codec string, kbps float64) {
	var best map[string]interface{}
	var bestBps float64
	for _, s := range streams {
		bps := parseFloatVal(strField(s, "bit_rate"))
		if bps > bestBps {
			bestBps = bps
			best = s
		}
	}
	if best == nil && len(streams) > 0 {
		best = streams[0]
		bestBps = parseFloatVal(strField(best, "bit_rate"))
	}
	if best == nil {
		return "", 0
	}
	codec = strField(best, "codec_name")
	return codec, bestBps / 1000
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// pickVideoBpsForPicture prefers the video track bitrate when it looks valid (>= ~500 kbps),
// otherwise falls back to container bitrate from file size or overall bit rate (sniffer / incomplete moov).
func pickVideoBpsForPicture(streamBps, calcBps, overallBps float64) float64 {
	const minCredibleVideo = 500_000
	if streamBps >= minCredibleVideo {
		return streamBps
	}
	if calcBps > 0 {
		return calcBps
	}
	if overallBps > 0 {
		return overallBps
	}
	return streamBps
}
