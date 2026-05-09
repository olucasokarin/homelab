#!/bin/bash
# Script de Deploy Otimizado para Serviço Systemd (ARM64)
# Torrent Sniffer

SERVER_IP="rock"
SERVICE_NAME="torrent-sniffer"

echo "--- 🛠️  Iniciando Build & Deploy para ARM64 ---"

# 1. Compilação
export GOOS=linux
export GOARCH=arm64
echo "🔨 Compilando torrent-sniffer para o Rock 3A..."
go build -o torrent-sniffer-linux-arm64 .

if [ $? -eq 0 ]; then
    echo "✅ Build OK: torrent-sniffer-linux-arm64"
else
    echo "❌ FALHA NA COMPILAÇÃO!"
    exit 1
fi

# 2. Empacotamento (Inclui apenas o binário, a pasta static e o service)
echo "📦 Criando pacote de arquivos..."
tar -czf torrent-sniffer-deploy.tar.gz torrent-sniffer-linux-arm64 static/ torrent-sniffer.service

# 3. Deploy
echo "🚀 Enviando pacote para $SERVER_IP..."
scp torrent-sniffer-deploy.tar.gz $SERVER_IP:~/

if [ $? -eq 0 ]; then
    ssh -t $SERVER_IP "sudo systemctl stop $SERVICE_NAME; \
                                  mkdir -p ~/$SERVICE_NAME; \
                                  mv torrent-sniffer-deploy.tar.gz ~/$SERVICE_NAME/; \
                                  cd ~/$SERVICE_NAME && tar -xzf torrent-sniffer-deploy.tar.gz; \
                                  chmod +x torrent-sniffer-linux-arm64; \
                                  sudo systemctl start $SERVICE_NAME; \
                                  rm torrent-sniffer-deploy.tar.gz"
                                  
    echo "✅ DEPLOY E REINÍCIO CONCLUÍDOS COM SUCESSO!"
    echo "🚀 Status: http://$SERVER_IP:8081"
    rm torrent-sniffer-deploy.tar.gz
else
    echo "❌ FALHA NO SCP!"
    rm torrent-sniffer-deploy.tar.gz
    exit 1
fi
