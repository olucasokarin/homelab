package bot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rockchip-bot/config"
	"rockchip-bot/downloader"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	botAPI    *tgbotapi.BotAPI
	DlManager *downloader.Downloader
	ownerID   int64
)

func Start() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	ownerStr := os.Getenv("TELEGRAM_OWNER_ID")

	if token == "" || ownerStr == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN or TELEGRAM_OWNER_ID not set")
	}

	id, err := strconv.ParseInt(ownerStr, 10, 64)
	if err != nil {
		log.Fatalf("Invalid TELEGRAM_OWNER_ID: %v", err)
	}
	ownerID = id

	// FORCE Local Bot API Server to allow downloads > 20MB (up to 2GB)
	tgEndpoint := "http://localhost:8082/bot%s/%s"
	botAPI, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, tgEndpoint)
	if err != nil {
		log.Fatalf("❌ CRITICAL: Could not connect to Local Bot API Server at %s. Error: %v. (Is the Docker container up?)", tgEndpoint, err)
	}

	log.Printf("🚀 Authorized on account %s via LOCAL API SERVER", botAPI.Self.UserName)

	botAPI.Debug = false
	log.Printf("Authorized on account %s", botAPI.Self.UserName)

	DlManager = downloader.NewDownloader(botAPI, token, 20) // 20 concurrent downloads limit

	// Restore persistence
	DlManager.LoadState(func(chatID int64, msgID int, text string) {
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		botAPI.Send(edit)
	})

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := botAPI.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			// Security check
			if update.Message.From.ID != ownerID {
				log.Printf("Unauthorized access attempt from user ID: %d", update.Message.From.ID)
				continue
			}

			handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			if update.CallbackQuery.From.ID != ownerID {
				continue
			}
			handleCallback(update.CallbackQuery)
		}
	}
}

func handleMessage(msg *tgbotapi.Message) {
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			sendReply(msg.Chat.ID, msg.MessageID, "👋 Olá! Envie ou encaminhe vídeos para baixar.\n\nComandos:\n/status - Ver downloads\n/changepath - Mudar pasta raiz\n/browse - Escolher série/temporada\n/setfolder <nome> - Pasta Manual (Zero)\n/mkdir <nome> - Subpasta Relativa (Append)\n/pwd - Onde estou agora?\n/retry - Tentar falhos")
		case "status":
			status := DlManager.GetFullStatus()
			active, sub := config.GlobalConfig.GetStatus()
			text := fmt.Sprintf("📊 *Status*\n\nAtuais: %d\nFila: %d\nFalhas: %d\n\n📁 Pasta atual: %s\n📂 Subpasta: %s", len(status.Active), len(status.Queue), len(status.Failed), active, sub)
			sendReply(msg.Chat.ID, msg.MessageID, text)
		case "pwd":
			active, sub := config.GlobalConfig.GetStatus()
			fullPath := config.GlobalConfig.GetFinalDir()
			text := fmt.Sprintf("📍 *Onde estamos agora?*\n\nRaiz: `%s`\nSub: `%s`\n\n📁 *Diretório Final:* \n`%s`", active, sub, fullPath)
			sendReply(msg.Chat.ID, msg.MessageID, text)
		case "mkdir":
			subNew := strings.TrimSpace(msg.CommandArguments())
			if subNew == "" {
				sendReply(msg.Chat.ID, msg.MessageID, "⚠ Informe o nome da nova pasta para entrar nela.")
				return
			}
			_, subCurrent := config.GlobalConfig.GetStatus()
			if subCurrent != "" {
				subNew = subCurrent + "/" + subNew
			}
			config.GlobalConfig.SetSubFolder(subNew)

			// Forçar criação no disco para aparecer no /browse
			final := config.GlobalConfig.GetFinalDir()

			text := fmt.Sprintf("✅ <b>Pasta Criada e Selecionada!</b>\n\n📍 Caminho Relativo: <code>%s</code>\n📂 Caminho Absoluto: <code>%s</code>", subNew, final)

			keyb := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📂 Navegar nesta pasta", "browse_"+subNew),
				),
			)
			m := tgbotapi.NewMessage(msg.Chat.ID, text)
			m.ParseMode = "HTML"
			m.ReplyToMessageID = msg.MessageID
			m.ReplyMarkup = keyb
			botAPI.Send(m)
		case "retry":
			c := DlManager.RetryAll()
			sendReply(msg.Chat.ID, msg.MessageID, fmt.Sprintf("🔄 Retentando %d downloads falhos...", c))
		case "changepath":
			keyb := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🎥 Filmes", "path_filmes"),
					tgbotapi.NewInlineKeyboardButtonData("📺 Séries", "path_series"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🤖 Radarr", "path_radarr"),
					tgbotapi.NewInlineKeyboardButtonData("📡 Sonarr", "path_sonarr"),
				),
			)
			m := tgbotapi.NewMessage(msg.Chat.ID, "Selecione o destino raiz:")
			m.ReplyMarkup = keyb
			botAPI.Send(m)
		case "browse":
			log.Printf("[Command] /browse received")
			showBrowse(msg.Chat.ID, 0, "")
		case "setfolder":
			sub := strings.TrimSpace(msg.CommandArguments())
			if sub == "" {
				sendReply(msg.Chat.ID, msg.MessageID, "⚠ Informe o nome da subpasta. Exemplo:\n/setfolder Breaking Bad/Season 01\n\nPara limpar, use /setfolder root")
				return
			}
			if sub == "root" {
				sub = ""
			}
			config.GlobalConfig.SetSubFolder(sub)

			text := fmt.Sprintf("✅ *Subpasta configurada!*\n\n📂 Pasta: `%s`\n📍 Destino: `%s`\n\n_Dica: Você pode usar '/' para criar pastas dentro de pastas._", sub, config.GlobalConfig.GetFinalDir())
			sendReply(msg.Chat.ID, msg.MessageID, text)
		}
		return
	}

	// Handle video/document
	var fileID string
	var fileName string
	var fileSize int

	if msg.Video != nil {
		fileID = msg.Video.FileID
		fileName = msg.Video.FileName
		fileSize = msg.Video.FileSize
		if fileName == "" {
			fileName = fmt.Sprintf("video_%d.mp4", msg.Time().Unix())
		}
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		fileName = msg.Document.FileName
		fileSize = msg.Document.FileSize
	} else {
		return // Not a video
	}

	msgID := msg.MessageID
	channelTitle := ""
	if msg.ForwardFromChat != nil {
		channelTitle = msg.ForwardFromChat.Title
	}

	// 1. Aplicar regras de renomeação (Ex: Doctor Who UW Clássica)
	originalName := fileName
	fileName = applyRenamingRules(fileName, channelTitle)

	// Send initial status message
	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("📥 Fila (%s)...", fileName))
	reply.ReplyToMessageID = msgID
	sent, err := botAPI.Send(reply)
	if err == nil {
		msgID = sent.MessageID
	}

	// Create job callback
	updateFunc := func(mID int, text string) {
		edit := tgbotapi.NewEditMessageText(msg.Chat.ID, mID, text)
		botAPI.Send(edit)
	}

	job := downloader.DownloadJob{
		FileID:    fileID,
		FileName:  fileName,
		Original:  originalName,
		FileSize:  int(fileSize),
		ChatID:    msg.Chat.ID,
		MsgID:     msgID,
		UpdateMsg: updateFunc,
	}

	DlManager.AddJob(job)
}

func handleCallback(cb *tgbotapi.CallbackQuery) {
	log.Printf("[Callback] Data: %s", cb.Data)
	callback := tgbotapi.NewCallback(cb.ID, cb.Data)
	if _, err := botAPI.Request(callback); err != nil {
		log.Println(err)
	}

	chatId := cb.Message.Chat.ID
	msgId := cb.Message.MessageID

	if strings.HasPrefix(cb.Data, "path_") {
		p := strings.TrimPrefix(cb.Data, "path_")
		if config.GlobalConfig.SetActivePath(p) {
			edit := tgbotapi.NewEditMessageText(chatId, msgId, fmt.Sprintf("✅ Path principal alterado para: %s\nSubpasta foi resetada.", p))
			botAPI.Send(edit)
		}
	} else if strings.HasPrefix(cb.Data, "browse_") {
		rel := strings.TrimPrefix(cb.Data, "browse_")
		showBrowse(chatId, msgId, rel)
	} else if strings.HasPrefix(cb.Data, "select_") {
		rel := strings.TrimPrefix(cb.Data, "select_")
		config.GlobalConfig.SetSubFolder(rel)
		edit := tgbotapi.NewEditMessageText(chatId, msgId, fmt.Sprintf("✅ <b>Subpasta configurada via Navegação!</b>\n\n📍 Destino: <code>%s</code>", config.GlobalConfig.GetFinalDir()))
		edit.ParseMode = "HTML"
		botAPI.Send(edit)
	}
}

func showBrowse(chatID int64, msgID int, currentRel string) {
	active, _ := config.GlobalConfig.GetStatus()
	basePath := config.BasePaths[active]
	fullPath := filepath.Join(basePath, currentRel)

	log.Printf("[Browse] Request for %s/%s", active, currentRel)
	dirs, err := listSubDirs(fullPath)
	if err != nil {
		log.Printf("[Browse] Error reading %s: %v", fullPath, err)
		if msgID != 0 {
			botAPI.Send(tgbotapi.NewEditMessageText(chatID, msgID, "❌ Erro ao ler pastas: "+err.Error()))
		} else {
			sendReply(chatID, 0, "❌ Erro ao ler pastas: "+err.Error())
		}
		return
	}
	log.Printf("[Browse] Found %d directories", len(dirs))

	var rows [][]tgbotapi.InlineKeyboardButton

	// Add folders (limit to 10 for safety)
	for i, d := range dirs {
		if i >= 10 {
			break
		}
		data := d
		if currentRel != "" {
			data = currentRel + "/" + d
		}

		// Ensure callback data doesn't exceed 64 bytes
		if len("browse_"+data) > 64 {
			log.Printf("[Browse] Skipping folder (too long for callback): %s", data)
			continue
		}

		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📁 "+d, "browse_"+data),
		))
	}

	// Navigation Row
	var navRow []tgbotapi.InlineKeyboardButton
	if currentRel != "" {
		parent := filepath.Dir(currentRel)
		if parent == "." {
			parent = ""
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Voltar", "browse_"+parent))
	}
	navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("✅ Selecionar Aqui", "select_"+currentRel))
	rows = append(rows, navRow)

	keyb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	text := fmt.Sprintf("📂 <b>Navegando em:</b> <code>%s</code>\nRaiz: <code>%s</code>", currentRel, active)
	if currentRel == "" {
		text = fmt.Sprintf("📂 <b>Navegando na Raiz:</b> <code>%s</code>", active)
	}

	if msgID != 0 {
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &keyb
		_, err = botAPI.Send(edit)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "HTML"
		msg.ReplyMarkup = keyb
		_, err = botAPI.Send(msg)
	}

	if err != nil {
		log.Printf("[Browse] Error sending message: %v", err)
	}
}

func listSubDirs(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs, nil
}

func sendReply(chatID int64, replyTo int, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyTo
	msg.ParseMode = "Markdown"
	botAPI.Send(msg)
}
