#!/bin/bash
# Script de Deploy Centralizado - Rockchip Node & TeleBot

echo "🚀 Iniciando DEPLOY TOTAL do Home Server..."

# 1. Deploy do Rockchip Node (Dashboard + API Hardware)
echo ""
echo "📦 [1/2] Iniciando Deploy do Rockchip-Node..."
cd node-rockchip
./deploy.sh
if [ $? -ne 0 ]; then
    echo "❌ Erro no deploy do Rockchip-Node. Abortando deploy-all."
    exit 1
fi
cd ..

# 2. Deploy do TeleBot (Downloader)
echo ""
echo "📦 [2/3] Iniciando Deploy do TeleBot..."
cd bot-rockchip
./deploy.sh
if [ $? -ne 0 ]; then
    echo "❌ Erro no deploy do TeleBot."
    exit 1
fi
cd ..

# 3. Deploy do Torrent Sniffer (Metadata Engine)
echo ""
echo "📦 [3/3] Iniciando Deploy do Torrent Sniffer..."
cd torrent-sniffer
./deploy.sh
if [ $? -ne 0 ]; then
    echo "❌ Erro no deploy do Torrent Sniffer."
    exit 1
fi
cd ..

echo ""
echo "✨ --- DEPLOY DE TODOS OS SERVIÇOS CONCLUÍDO COM SUCESSO! --- ✨"
echo "🌐 Dashboard: http://192.168.1.110:5000"
echo "🤖 TeleBot API: http://192.168.1.110:5001"
echo "🔎 Torrent Sniffer: http://192.168.1.110:8081"
