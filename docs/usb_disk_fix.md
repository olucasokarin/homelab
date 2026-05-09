# Solução de Instabilidade nos Discos USB (Rock 3A)

Este documento registra o diagnóstico e a solução aplicada para resolver as desconexões frequentes do HD Externo (WdBlue) que causavam erros de leitura no Jellyfin e falhas no sistema de arquivos.

## 1. O Problema
O disco externo `/dev/sda` (WdBlue 1TB) apresentava desconexões repentinas sob carga (ex: streaming no Jellyfin ou downloads no qBittorrent). 
- **Sintomas:** Erros de I/O no log (`dmesg`), mudança de nome do dispositivo (ex: de `sda` para `sdc`), e erro "Invalid data found when processing input" no FFmpeg.

## 2. Diagnóstico Técnico
Foram identificadas duas causas principais:
1.  **Picos de Consumo:** O consumo de pico de dois HDDs mecânicos superava a capacidade de entrega da porta USB do Rock 3A (limitada pela fonte de 5V/3A).
2.  **Incompatibilidade UAS:** O chip **Realtek RTL9201** das cases USB apresenta instabilidade com o driver **UAS (USB Attached SCSI)** do Linux em arquitetura ARM, resultando em resets de hardware sob alto tráfego de I/O.

## 3. Soluções Aplicadas

### A. Estabilidade Elétrica
- **Ação:** Substituição da case por um modelo com **alimentação externa (DC 12V 2A)**.
- **Resultado:** O disco deixou de depender da energia da porta USB do Rock 3A, eliminando quedas por subtensão (*under-voltage*).

### B. Correção de Software (USB Quirk)
- **Ação:** Desativação do protocolo UAS para o ID de dispositivo `0bda:9201`, forçando o uso do driver `usb-storage` estável.
- **Configuração:** Editado o arquivo `/boot/armbianEnv.txt` no servidor.
- **Parâmetros adicionados:**
  - No `extraargs`: `usb-storage.quirks=0bda:9201:u`
  - No `usbstoragequirks`: `,0x0bda:0x9201:u`

## 4. Comandos de Verificação
Para validar se a correção está ativa após o reboot:

```bash
# Verificar se o driver em uso é o usb-storage (e não uas)
lsusb -t

# Verificar a saúde física do disco (deve retornar 0 em erros)
sudo smartctl -d sat -a /dev/sda | grep -iE "reallocated|pending|uncorrectable"

# Monitorar erros de I/O em tempo real
dmesg -w | grep -iE "usb|sd|error"
```

## 5. Estado Atual
- **Discos:** `/dev/sda` (WdBlue) e `/dev/sdb1` (Blue) operando com o driver `usb-storage`.
- **Status:** Estável.
