#!/bin/bash
# Script de Deploy Otimizado para Serviço Systemd (ARM64) - TeleBot

SERVER_IP="rock"

echo "--- 🛠️  Iniciando Build & Deploy para ARM64 (TeleBot) ---"

# 1. Compilação
export GOOS=linux
export GOARCH=arm64
echo "🔨 Compilando rockchip-bot para o Rock 3A..."
go build -o telebot-linux-arm64 .

if [ $? -eq 0 ]; then
    echo "✅ Build OK: telebot-linux-arm64"
else
    echo "❌ FALHA NA COMPILAÇÃO!"
    exit 1
fi

# 2. Empacotamento
echo "📦 Criando pacote de arquivos..."
# Apenas o binário e o arquivo de serviço. O config.json é gerado se não existir.
tar -czf telebot-deploy.tar.gz telebot-linux-arm64 rockchip-bot.service

# 3. Deploy
echo "🚀 Enviando pacote para $SERVER_IP..."
scp telebot-deploy.tar.gz $SERVER_IP:~/

if [ $? -eq 0 ]; then
    ssh -t $SERVER_IP "sudo systemctl stop rockchip-bot; \
                                  tar -xzf telebot-deploy.tar.gz; \
                                  chmod +x telebot-linux-arm64; \
                                  sudo systemctl start rockchip-bot; \
                                  rm telebot-deploy.tar.gz"
                                  
    echo "✅ DEPLOY E REINÍCIO CONCLUÍDOS COM SUCESSO!"
    rm telebot-deploy.tar.gz
else
    echo "❌ FALHA NO SCP!"
    rm telebot-deploy.tar.gz
    exit 1
fi
