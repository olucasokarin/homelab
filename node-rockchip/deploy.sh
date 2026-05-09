#!/bin/bash
# Script de Deploy Otimizado para Serviço Systemd (ARM64)

SERVER_IP="rock"

echo "--- 🛠️  Iniciando Build & Deploy para ARM64 ---"

# 1. Compilação
export GOOS=linux
export GOARCH=arm64
echo "🔨 Compilando rockchip-node para o Rock 3A..."
go build -o rockchip-node-linux-arm64 .

if [ $? -eq 0 ]; then
    echo "✅ Build OK: rockchip-node-linux-arm64"
else
    echo "❌ FALHA NA COMPILAÇÃO!"
    exit 1
fi

# 2. Empacotamento (Inclui apenas o binário e a pasta static)
echo "📦 Criando pacote de arquivos..."
tar -czf rockchip-node-deploy.tar.gz rockchip-node-linux-arm64 static/

# 3. Deploy
echo "🚀 Enviando pacote para $SERVER_IP..."
scp rockchip-node-deploy.tar.gz $SERVER_IP:~/

if [ $? -eq 0 ]; then
    ssh -t $SERVER_IP "sudo systemctl stop rockchip-node; \
                                  tar -xzf rockchip-node-deploy.tar.gz; \
                                  chmod +x rockchip-node-linux-arm64; \
                                  sudo systemctl start rockchip-node; \
                                  rm rockchip-node-deploy.tar.gz"
                                  
    echo "✅ DEPLOY E REINÍCIO CONCLUÍDOS COM SUCESSO!"
    echo "🚀 Status: http://$SERVER_IP:5000"
    rm rockchip-node-deploy.tar.gz
else
    echo "❌ FALHA NO SCP!"
    rm rockchip-node-deploy.tar.gz
    exit 1
fi
