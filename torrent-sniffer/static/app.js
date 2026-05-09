document.addEventListener('DOMContentLoaded', () => {
    const magnetInput = document.getElementById('magnet-input');
    const sniffBtn = document.getElementById('sniff-btn');
    const loader = document.getElementById('loader');
    const historyList = document.getElementById('history-list');
    const modal = document.getElementById('result-modal');
    const modalBody = document.getElementById('modal-body');
    const closeModal = document.querySelector('.close-modal');
    const flushBtn = document.getElementById('flush-btn');

    const queueContainer = document.getElementById('queue-container');

    // Fetch history and queue on startup
    fetchHistory();
    fetchQueue();
    setInterval(fetchQueue, 5000); // Poll tracking 5s

    let batchResults = [];
    let batchTotal = 0;
    let batchCompleted = 0;
    const batchContainer = document.getElementById('batch-container');
    const batchProgress = document.getElementById('batch-progress');
    const batchBestSoFar = document.getElementById('batch-best-so-far');

    function updateBatchUI() {
        if (batchTotal === 0) {
            batchContainer.style.display = 'none';
            return;
        }
        batchContainer.style.display = 'block';
        batchProgress.innerText = `${batchCompleted}/${batchTotal}`;

        if (batchResults.length > 0) {
            let best = null;
            let highestScore = -1;
            batchResults.forEach(r => {
                let probeObj = r.probe;
                if (typeof probeObj === 'string') {
                    try { probeObj = JSON.parse(probeObj); } catch (e) { }
                }
                const analysis = performExpertAnalysis(probeObj, r.file_size);
                if (analysis && analysis.avg > highestScore) {
                    highestScore = analysis.avg;
                    best = { result: r, analysis: analysis };
                }
            });

            if (best) {
                batchBestSoFar.innerHTML = `
                    <span style="color: #3fb950; font-weight: bold;">Melhor até agora:</span> ${best.result.torrent_name || 'Desconhecido'} 
                    <span style="color: ${best.analysis.vColor}; font-size: 0.8rem; background: rgba(0,0,0,0.2); padding: 2px 6px; border-radius: 4px;">Score: ${best.analysis.avg}</span>
                    <span style="color: ${best.analysis.bitrateColor}; font-size: 0.8rem; background: rgba(0,0,0,0.2); padding: 2px 6px; border-radius: 4px; margin-left:5px;">${best.analysis.bitrateMbps.toFixed(1)} Mbps</span>
                `;
            }
        }
        
        if (batchCompleted === batchTotal && batchTotal > 0) {
            setTimeout(() => {
                fetchHistory(); // Atualiza backend UI
                let best = null;
                let highestScore = -1;
                batchResults.forEach(r => {
                    let probeObj = r.probe;
                    if (typeof probeObj === 'string') {
                        try { probeObj = JSON.parse(probeObj); } catch (e) { }
                    }
                    const analysis = performExpertAnalysis(probeObj, r.file_size);
                    if (analysis && analysis.avg > highestScore) {
                        highestScore = analysis.avg;
                        best = { result: r, analysis: analysis };
                    }
                });
                if (best) {
                    alert('Lote concluído! Exibindo o melhor resultado da análise.');
                    showResult(best.result);
                }
                // Reset após mostrar info
                batchTotal = 0;
                batchCompleted = 0;
                batchResults = [];
            }, 500);
        }
    }

    sniffBtn.addEventListener('click', async () => {
        const text = magnetInput.value.trim();
        if (!text) return alert('Por favor, insira um link magnético.');
        
        const magnets = text.split('\n').map(l => l.trim()).filter(l => l.startsWith('magnet:?'));
        if (magnets.length === 0) {
            return alert('Nenhum link válido (deve começar com magnet:?)');
        }

        magnetInput.value = '';

        if (magnets.length > 1) {
            batchTotal = magnets.length;
            batchCompleted = 0;
            batchResults = [];
            batchBestSoFar.innerHTML = 'Aguardando análises...';
            updateBatchUI();
        }

        magnets.forEach(magnet => {
            fetch('/sniff', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ magnet })
            }).then(async response => {
                if (!response.ok) throw new Error(await response.text());
                const result = await response.json();
                
                if (batchTotal > 1) {
                    batchResults.push(result);
                    batchCompleted++;
                    updateBatchUI();
                } else {
                    showResult(result);
                    fetchHistory();
                }
            }).catch(err => {
                console.error('Erro na análise: ' + err.message);
                if (batchTotal > 1) {
                    batchCompleted++;
                    updateBatchUI();
                } else {
                    alert('Erro na análise: ' + err.message);
                }
            });
        });

        // Atualiza a fila instantaneamente para mostrar "Aguardando..."
        setTimeout(fetchQueue, 500);
    });

    async function fetchQueue() {
        try {
            const res = await fetch('/queue');
            const data = await res.json();
            renderQueue(data || []);
        } catch (e) {
            console.error('Queue poll error:', e);
        }
    }

    function renderQueue(tasks) {
        queueContainer.innerHTML = '';
        tasks.forEach(t => {
            const el = document.createElement('div');
            el.className = 'queue-card';
            el.style = 'background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); padding: 10px 15px; border-radius: 8px; display: flex; flex-direction: column; gap: 5px;';
            el.innerHTML = `
                <div style="display:flex; justify-content:space-between; align-items:center;">
                    <div style="display:flex; flex-direction:column; max-width:70%;">
                        <span style="font-size: 0.85rem; font-weight:600; color: #fff; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">${t.Name || 'Aguardando Metadados...'}</span>
                        <span style="font-size: 0.75rem; font-family: monospace; color: #58a6ff; margin-top:3px;">Progresso: ${t.DownloadedMB > 0 ? `(${t.DownloadedMB.toFixed(1)} MB)` : '0.0 MB'}</span>
                    </div>
                    <div style="display:flex; flex-direction:column; align-items:flex-end; gap:6px;">
                        <span style="font-size: 0.7rem; background: #6366f1; padding: 2px 8px; border-radius: 10px; color: white;">${t.Status}</span>
                        <button onclick="cancelTask('${t.ID}')" style="background:rgba(248,81,73,0.15); border:1px solid rgba(248,81,73,0.3); color:#f85149; padding:2px 8px; border-radius:6px; cursor:pointer; font-size:0.7rem;">Cancelar</button>
                    </div>
                </div>
            `;
            queueContainer.appendChild(el);
        });
    }

    window.cancelTask = async (id) => {
        if (!confirm('Deseja cancelar esta análise?')) return;
        try {
            await fetch('/cancel?id=' + id, { method: 'POST' });
            fetchQueue();
        } catch (e) {
            console.error('Cancel error:', e);
        }
    }

    async function fetchHistory() {
        try {
            const res = await fetch('/history');
            const data = await res.json();
            renderHistory(data);
        } catch (err) {
            console.error('Falha ao buscar histórico', err);
        }
    }

    if (flushBtn) {
        flushBtn.onclick = async () => {
            if (!confirm('Tem certeza que deseja APAGAR TODO o banco de dados? Isso é irreversível.')) return;
            try {
                await fetch('/flush', { method: 'POST' });
                fetchHistory();
            } catch (e) {
                alert('Erro ao apagar: ' + e.message);
            }
        };
    }

    window.deleteHistoryItem = async (id, e) => {
        e.stopPropagation(); // Evita abrir o modal
        if (!confirm('Deletar esta análise específica?')) return;
        try {
            await fetch('/delete?id=' + id, { method: 'POST' });
            fetchHistory();
        } catch (err) {
            alert('Erro: ' + err.message);
        }
    };

    function renderHistory(items) {
        if (!items) items = [];
        historyList.innerHTML = items.length ? '' : '<p style="color:#8b949e">Nenhuma análise realizada ainda.</p>';
        console.log("Renderizando histórico: ", items.length, " itens.");

        items.forEach((item, index) => {
            const div = document.createElement('div');
            div.className = 'sniff-card';
            div.style.position = 'relative';

            const sizeGB = (item.file_size / (1024 ** 3)).toFixed(2);
            
            // Força a extração segura do bitrate
            let bitrateStr = '? Mbps';
            let bitrateColor = '#8b949e';
            
            try {
                let probeObj = item.probe;
                // Double-parse safety for strings or objects
                if (typeof probeObj === 'string') {
                    try { 
                        probeObj = JSON.parse(probeObj); 
                        if (typeof probeObj === 'string') probeObj = JSON.parse(probeObj); 
                    } catch (e) {}
                }
                
                const analysis = performExpertAnalysis(probeObj, item.file_size);
                if (analysis && analysis.bitrateMbps) {
                    bitrateStr = `${analysis.bitrateMbps.toFixed(1)} Mbps`;
                    bitrateColor = analysis.bitrateColor;
                }
            } catch (err) {
                console.warn("Erro ao analisar item:", item.torrent_name, err);
            }

            let fetchInfo = '';
            if (item.downloaded_bytes > 0) {
                const mb = (item.downloaded_bytes / 1024 / 1024).toFixed(1);
                fetchInfo = item.tail_fallback ?
                    `<span style="margin-top:5px;display:inline-block;font-size:0.65rem;background:rgba(210,153,34,0.1);color:#d29922;padding:2px 6px;border-radius:4px;">Lidos ${mb}MB (Do Fim)</span>` :
                    `<span style="margin-top:5px;display:inline-block;font-size:0.65rem;background:rgba(63,185,80,0.1);color:#3fb950;padding:2px 6px;border-radius:4px;">Lidos ${mb}MB (Do Início)</span>`;
            }

            div.innerHTML = `
                <div class="delete-badge" onclick="deleteHistoryItem(${item.id}, event)" title="Deletar">✕</div>
                <h3>${item.torrent_name || 'Desconhecido'}</h3>
                <div class="meta" style="gap: 10px;">
                    <span style="display:flex; align-items:center;">
                        ${sizeGB} GB
                        <span class="bitrate-tag" style="color:${bitrateColor}; font-weight:600; margin-left:8px; background:rgba(255,255,255,0.05); padding:1px 6px; border-radius:4px; font-size:0.75rem; border: 1px solid ${bitrateColor}22;">${bitrateStr}</span>
                    </span>
                    <span class="badge">${item.probe_tool}</span>
                    <span class="health-badge">
                        <span class="dot seeds"></span> ${item.seeds || 0}
                        <span class="dot peers"></span> ${item.peers || 0}
                    </span>
                </div>
                ${fetchInfo}
                <div style="font-size:0.7rem; color:#484f58; margin-top:8px;">${new Date(item.created_at).toLocaleString()}</div>
            `;

            div.onclick = () => showResult(item, items, index);
            historyList.appendChild(div);
        });
    }

    function showResult(result, contextArray = null, currentIndex = -1) {
        let probeObj = result.probe;
        if (typeof probeObj === 'string') {
            try { probeObj = JSON.parse(probeObj); } catch (e) { }
        }

        const analysis = performExpertAnalysis(probeObj, result.file_size);

        let navHtml = '';
        if (contextArray && contextArray.length > 1 && currentIndex >= 0) {
            const hasPrev = currentIndex > 0;
            const hasNext = currentIndex < contextArray.length - 1;
            navHtml = `
                <div style="display:flex; gap: 8px; margin-bottom: 15px; align-items:center; background: rgba(255,255,255,0.05); padding: 8px 12px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.1);">
                    <button id="nav-prev-btn" class="magnet-btn" style="margin:0; padding:4px 10px; font-size:12px; ${hasPrev ? 'background:#30363d;' : 'background:transparent; border:1px solid #30363d; opacity:0.5; cursor:not-allowed;'}" ${hasPrev ? '' : 'disabled'}>&larr; Anterior</button>
                    <span style="color:#8b949e; font-size:13px; font-weight:bold; flex:1; text-align:center;">Navegação: ${currentIndex + 1} de ${contextArray.length}</span>
                    <button id="nav-next-btn" class="magnet-btn" style="margin:0; padding:4px 10px; font-size:12px; ${hasNext ? 'background:#30363d;' : 'background:transparent; border:1px solid #30363d; opacity:0.5; cursor:not-allowed;'}" ${hasNext ? '' : 'disabled'}>Próximo &rarr;</button>
                </div>
            `;
        }

        modalBody.innerHTML = `
            ${navHtml}
            <div style="display:flex; justify-content:space-between; align-items:flex-start; gap:20px;">
                <div style="flex:1;">
                    <h2 style="color:#fff; margin-bottom:5px;">${result.torrent_name || 'Detalhes da Mídia'}</h2>
                    <div style="display:flex; gap:12px; margin-bottom:8px;">
                        <span class="h-stat"><i class="h-dot seeds"></i> Seeds: <b>${result.seeds || 0}</b></span>
                        <span class="h-stat"><i class="h-dot peers"></i> Peers: <b>${result.peers || 0}</b></span>
                    </div>
                    <p style="color:#8b949e; font-size:13px; margin-bottom:15px; word-break:break-all;">Arquivo Interno: ${result.file_name}</p>
                </div>
                <div style="display:flex; flex-direction:column; gap:8px; flex-shrink:0;">
                    ${result.magnet ? `<button id="retest-btn" class="magnet-btn" style="background:#238636; margin:0; width:100%;">Retestar</button>` : ''}
                    ${result.magnet ? `<button id="copy-magnet" class="magnet-btn" style="margin:0; width:100%;">Copiar Link</button>` : ''}
                    <button id="copy-raw-btn" class="magnet-btn" style="margin:0; width:100%; background:#1f6feb;">Copiar Info</button>
                </div>
            </div>
            <hr style="border:0; border-top:1px solid #30363d; margin: 15px 0;">
            ${analysis ? renderExpertBlock(analysis) : ''}
            
            <h3 style="color:#8b949e; margin-top:20px; font-size:14px; font-weight:normal;">Relatório Raw (${result.probe_tool})</h3>
            <div class="mediainfo-raw">${fmtRaw(probeObj, result.probe_tool)}</div>
        `;
        modal.classList.remove('hidden');

        const clipBtn = document.getElementById('copy-magnet');
        if (clipBtn && result.magnet) {
            clipBtn.onclick = () => {
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(result.magnet);
                } else {
                    const ta = document.createElement("textarea");
                    ta.value = result.magnet;
                    ta.style.position = "fixed";
                    document.body.appendChild(ta);
                    ta.select();
                    try { document.execCommand('copy'); } catch (e) { }
                    document.body.removeChild(ta);
                }
                clipBtn.innerText = 'Copiado!';
                setTimeout(() => clipBtn.innerText = 'Copiar', 2000);
            };
        }

        const retestBtn = document.getElementById('retest-btn');
        if (retestBtn && result.magnet) {
            retestBtn.onclick = () => {
                modal.classList.add('hidden');
                magnetInput.value = result.magnet;
                sniffBtn.click();
            };
        }

        const prevBtn = document.getElementById('nav-prev-btn');
        if (prevBtn && !prevBtn.disabled) {
            prevBtn.onclick = () => showResult(contextArray[currentIndex - 1], contextArray, currentIndex - 1);
        }
        
        const nextBtn = document.getElementById('nav-next-btn');
        if (nextBtn && !nextBtn.disabled) {
            nextBtn.onclick = () => showResult(contextArray[currentIndex + 1], contextArray, currentIndex + 1);
        }

        const copyRawBtn = document.getElementById('copy-raw-btn');
        if (copyRawBtn) {
            copyRawBtn.onclick = () => {
                const rawText = fmtRaw(probeObj, result.probe_tool);
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(rawText);
                } else {
                    const ta = document.createElement("textarea");
                    ta.value = rawText;
                    ta.style.position = "fixed";
                    document.body.appendChild(ta);
                    ta.select();
                    try { document.execCommand('copy'); } catch (e) { }
                    document.body.removeChild(ta);
                }
                copyRawBtn.innerText = 'Copiado!';
                setTimeout(() => copyRawBtn.innerText = 'Copiar Info', 2000);
            };
        }
    }

    closeModal.onclick = () => modal.classList.add('hidden');
    window.onclick = (event) => {
        if (event.target == modal) modal.classList.add('hidden');
    };

    // ==========================================
    // EXPERT ANALYSIS ENGINE
    // ==========================================
    const TV = { name: 'TCL 55C6K', peakNits: 1300, supportsDV: true, supportsHDRPlus: true, supportsHDR10: true };

    function num(v) {
        if (v == null) return 0;
        if (typeof v === 'number') return v;
        // Remove espaços (comum no mediainfo) e converte para string limpa
        const clean = String(v).replace(/\s/g, '').replace(/,/g, '');
        const match = clean.match(/[\d.]+/);
        return match ? parseFloat(match[0]) : 0;
    }

    function getFFprobeField(s, fields) {
        for (let f of fields) if (s[f]) return s[f];
        if (s.tags) { for (let f of fields) if (s.tags[f]) return s.tags[f]; }
        return '';
    }

    function performExpertAnalysis(infoObj, fileSize) {
        let V = null, A = null, audios = [];
        let formatName = '', isFfprobe = Array.isArray(infoObj?.streams);

        if (isFfprobe) {
            const general = infoObj.format || {};
            V = infoObj.streams.find(s => s.codec_type === 'video');
            audios = infoObj.streams.filter(s => s.codec_type === 'audio').map(s => ({
                Format: s.codec_name,
                Language: getFFprobeField(s, ['language']),
                Title: getFFprobeField(s, ['title']),
                Channels: s.channels,
                BitRate: getFFprobeField(s, ['bit_rate', 'BPS'])
            }));

            V = V ? {
                BitRate: getFFprobeField(V, ['bit_rate', 'BPS']) || general.bit_rate,
                Width: V.width,
                Height: V.height,
                FrameRate: eval(V.r_frame_rate || '0') || 0,
                Format: V.codec_name,
                BitDepth: V.bits_per_raw_sample || V.bits_per_sample,
                ColorPrimaries: V.color_primaries,
                transfer_characteristics: V.color_transfer,
                HDR_Format: getFFprobeField(V, ['hdr_format'])
            } : null;
        } else {
            const tracks = infoObj?.media?.track || [];
            const general = tracks.find(t => t['@type'] === 'General') || {};
            V = tracks.find(t => t['@type'] === 'Video');
            audios = tracks.filter(t => t['@type'] === 'Audio');

            if (V && !V.BitRate && general.OverallBitRate) {
                V.BitRate = general.OverallBitRate;
            }
        }

        if (!V) return null;

        let realDurationSecs = 0;
        if (isFfprobe && infoObj?.format?.duration) {
            realDurationSecs = parseFloat(infoObj.format.duration);
        } else if (!isFfprobe && infoObj?.media?.track) {
            const gen = infoObj.media.track.find(t => t['@type'] === 'General');
            if (gen && gen.Duration) realDurationSecs = parseFloat(gen.Duration);
        }

        let calculatedBitrate = 0;
        if (realDurationSecs > 0 && fileSize > 0) {
            calculatedBitrate = (fileSize * 8) / realDurationSecs; // bps
        }

        // Sempre preferimos o bitrate calculado pois mediainfo/ffprobe erram feio o bitrate de arquivos truncados (baixados parcialmente)
        if (calculatedBitrate > 0) {
            V.BitRate = calculatedBitrate;
        } else if (!V.BitRate || num(V.BitRate) < 100000) {
            V.BitRate = 0;
        }

        // ... existing bitrate logic ...
        const bitrateMbps = num(V.BitRate) / 1000000;
        const width = num(V.Width);
        const height = num(V.Height);
        const fps = num(V.FrameRate);
        const codec = V.Format || '';
        const fileSizeGB = fileSize / (1024 ** 3);

        const is4K = width >= 3840;
        let bitrateClass, bitrateScore, bitrateNote, bitrateColor;
        if (is4K) {
            if (bitrateMbps > 60) { bitrateClass = 'Remux UHD'; bitrateScore = 100; bitrateColor = '#a78bfa'; bitrateNote = 'Stream original sem compressão extra.'; }
            else if (bitrateMbps > 30) { bitrateClass = 'Alta Fidelidade'; bitrateScore = 90; bitrateColor = '#3fb950'; bitrateNote = 'Aproveita bem o painel 4K.'; }
            else if (bitrateMbps > 15) { bitrateClass = 'WEB Alta Fidelidade'; bitrateScore = 75; bitrateColor = '#58a6ff'; bitrateNote = 'Aproveitamento sólido.'; }
            else if (bitrateMbps > 10) { bitrateClass = 'WEB Médio'; bitrateScore = 55; bitrateColor = '#d29922'; bitrateNote = 'Bitrate padrão de streaming.'; }
            else { bitrateClass = 'Comprimido'; bitrateScore = 35; bitrateColor = '#f85149'; bitrateNote = `${bitrateMbps.toFixed(1)} Mbps — Baixo para 4K.`; }
        } else {
            if (bitrateMbps > 30) { bitrateClass = 'Remux 1080p'; bitrateScore = 100; bitrateColor = '#a78bfa'; bitrateNote = 'Referência para 1080p.'; }
            else if (bitrateMbps > 15) { bitrateClass = 'Alta Fidelidade'; bitrateScore = 90; bitrateColor = '#3fb950'; bitrateNote = '1080p de alta qualidade.'; }
            else if (bitrateMbps > 8) { bitrateClass = 'WEB Alta Fidelidade'; bitrateScore = 70; bitrateColor = '#58a6ff'; bitrateNote = 'Padrão FHD WEB-DL.'; }
            else { bitrateClass = 'Comprimido'; bitrateScore = 40; bitrateColor = '#d29922'; bitrateNote = 'Compressão notável.'; }
        }

        const bitDepth = num(V.BitDepth || V.Bit_depth || '8');
        const colorPrim = String(V.ColorPrimaries || V.color_primaries || '');
        const hdrRaw = (String(V.HDR_Format || '') + ' ' + String(V.HDR_Format_Compatibility || '') + ' ' + String(V.HDR_Format_String || '') + ' ' + String(V.transfer_characteristics || '') + ' ' + String(V.colour_transfer || '')).toLowerCase();

        let colorScore = bitDepth >= 10 ? 45 : 10;
        if (colorPrim.includes('2020')) colorScore += 35;
        else if (colorPrim.includes('P3') || colorPrim.includes('p3')) colorScore += 25;
        colorScore = Math.min(100, colorScore + 20);
        const colorColor = colorScore >= 80 ? '#3fb950' : colorScore >= 55 ? '#58a6ff' : '#d29922';

        const isDV = hdrRaw.includes('dolby vision') || hdrRaw.includes('dv');
        const isHDR10P = hdrRaw.includes('hdr10+') || hdrRaw.includes('st 2094') || hdrRaw.includes('2094');
        const isHDR10 = hdrRaw.includes('hdr10') || hdrRaw.includes('pq') || hdrRaw.includes('smpte2084') || hdrRaw.includes('2086');

        // Extract DV profile number
        let dvProfile = '';
        if (isDV) {
          const dvMatch = hdrRaw.match(/dvhe\.(\d+)|dvh1\.(\d+)|profile\s*[:.]?\s*(\d+)/);
          if (dvMatch) {
            const pNum = dvMatch[1] || dvMatch[2] || dvMatch[3];
            dvProfile = ' P' + pNum;
            // Detect sub-profiles (e.g. P8.1 via cross-compatibility layer)
            if (pNum === '8' && (hdrRaw.includes('8.1') || hdrRaw.includes('cross'))) {
              dvProfile = ' P8.1';
            }
          }
        }

        let hdrScore = 15, hdrLabel = 'SDR', hdrColor = '#8b949e';
        if (isDV && isHDR10P) { hdrScore = 100; hdrLabel = 'DV' + dvProfile + ' + HDR10+'; hdrColor = '#d29922'; }
        else if (isDV) { hdrScore = 90; hdrLabel = 'Dolby Vision' + dvProfile; hdrColor = '#d29922'; }
        else if (isHDR10P) { hdrScore = 85; hdrLabel = 'HDR10+'; hdrColor = '#a78bfa'; }
        else if (isHDR10) { hdrScore = 65; hdrLabel = 'HDR10'; hdrColor = '#58a6ff'; }

        // Audio Analysis
        let audioScore = 15, audioLabel = 'Sem Áudio', audioColor = '#8b949e', audioNote = 'Nenhuma trilha encontrada.';
        if (audios.length > 0) {
            let maxCh = 0, bestCodec = '', isAtmos = false;
            for (let a of audios) {
                let text = String(a.Format || '') + String(a.Format_Commercial_IfAny || '') + String(a.Format_AdditionalFeatures || '');
                let ch = num(a.Channels || a['Channel(s)'] || 2);
                if (ch > maxCh) maxCh = ch;
                if (text.toLowerCase().includes('atmos') || text.toLowerCase().includes('joc')) isAtmos = true;
                if (text.toLowerCase().includes('truehd') || text.toLowerCase().includes('dts-hd') || text.toLowerCase().includes('dts')) bestCodec = a.Format;
            }
            if (!bestCodec) bestCodec = audios[0].Format;

            audioLabel = `${maxCh}ch ${bestCodec}`;
            if (isAtmos) {
                audioScore = 100; audioLabel = 'Dolby Atmos / 7.1+'; audioColor = '#a78bfa'; audioNote = 'Imersão espacial premium detectada.';
            } else if (maxCh >= 7) {
                audioScore = 90; audioLabel = '7.1 Surround'; audioColor = '#3fb950'; audioNote = 'Mixagem de alta definição com surround pleno.';
            } else if (maxCh >= 5) {
                audioScore = 75; audioLabel = '5.1 Surround'; audioColor = '#58a6ff'; audioNote = 'Acústica de cinema padrão.';
            } else {
                audioScore = 40; audioLabel = 'Stereo (2.0)'; audioColor = '#d29922'; audioNote = 'Áudio estéreo simples. Limitado.';
            }
        }

        const avg = Math.round((bitrateScore + colorScore + hdrScore + audioScore) / 4);
        let verdict, vColor, vNote;
        if (avg >= 85) { verdict = 'Referência'; vColor = '#a78bfa'; vNote = 'Encode espetacular A/V.'; }
        else if (avg >= 70) { verdict = 'Alta'; vColor = '#3fb950'; vNote = 'Ótimo para colecionar.'; }
        else if (avg >= 50) { verdict = 'Média-Alta'; vColor = '#58a6ff'; vNote = 'Bom equilíbrio tamanho/qualidade.'; }
        else if (avg >= 35) { verdict = 'Média'; vColor = '#d29922'; vNote = 'Sólido, mas compressão notável.'; }
        else { verdict = 'Baixa'; vColor = '#f85149'; vNote = 'Mídia com baixa resolução/bitrate.'; }

        return {
            bitrateMbps, bitrateClass, bitrateScore, bitrateNote, bitrateColor, width, height, fps, codec, fileSizeGB,
            bitDepth, colorPrim, colorScore, colorColor,
            hdrLabel, hdrScore, hdrColor,
            audioLabel, audioScore, audioColor, audioNote,
            avg, verdict, vColor, vNote
        };
    }

    function renderExpertBlock(a) {
        const pct = s => `${Math.round(s)}%`;

        return `
        <div class="expert-block">
          <div class="expert-header">
            <span class="expert-header-title">Veredito de Engenharia</span>
            <span class="verdict-pill" style="background:${a.vColor}22;border:1px solid ${a.vColor}55;color:${a.vColor}">${a.verdict}</span>
          </div>

          <div class="expert-grid">
            <div class="expert-criterion">
              <div class="crit-label">Eficiência de Bitrate</div>
              <div class="crit-value">
                <span style="color:${a.bitrateColor}">${a.bitrateClass}</span>
                <span style="color:#8b949e;font-size:0.72rem;font-weight:400">${a.bitrateMbps.toFixed(1)} Mbps</span>
              </div>
              <div class="score-bar-wrap"><div class="score-bar-fill" style="width:${pct(a.bitrateScore)};background:${a.bitrateColor}"></div></div>
              <div class="crit-note">${a.bitrateNote}</div>
              <div class="crit-note" style="margin-top:4px;color:#8b949e">${a.width}×${a.height} · ${a.codec} · ${a.fileSizeGB.toFixed(1)} GB</div>
            </div>

            <div class="expert-criterion">
              <div class="crit-label">Profundidade & Cor</div>
              <div class="crit-value"><span style="color:${a.colorColor}">${a.bitDepth}-bit · ${a.colorPrim || 'N/A'}</span></div>
              <div class="score-bar-wrap"><div class="score-bar-fill" style="width:${pct(a.colorScore)};background:${a.colorColor}"></div></div>
              <div class="crit-note">Indicativo de qualidade do espaço de cores.</div>
            </div>

            <div class="expert-criterion">
              <div class="crit-label">Implementação HDR</div>
              <div class="crit-value"><span style="color:${a.hdrColor}">${a.hdrLabel}</span></div>
              <div class="score-bar-wrap"><div class="score-bar-fill" style="width:${pct(a.hdrScore)};background:${a.hdrColor}"></div></div>
              <div class="crit-note">Mapeamento de tons HDR detectado via Meta.</div>
            </div>

            <div class="expert-criterion">
              <div class="crit-label">Aproveitamento Sonoro</div>
              <div class="crit-value" style="color:${a.audioColor}">${a.audioLabel}</div>
              <div class="score-bar-wrap"><div class="score-bar-fill" style="width:${pct(a.audioScore)};background:${a.audioColor}"></div></div>
              <div class="crit-note">${a.audioNote}</div>
            </div>
          </div>

          <div class="verdict-summary">
            <div>
              <div class="verdict-text" style="margin-bottom:6px">Score geral: ${a.avg}/100</div>
              <div class="verdict-score-row">
                <div class="verdict-score-seg"><div class="verdict-score-seg-fill" style="width:${pct(a.bitrateScore)};background:${a.bitrateColor}"></div></div>
                <div class="verdict-score-seg"><div class="verdict-score-seg-fill" style="width:${pct(a.colorScore)};background:${a.colorColor}"></div></div>
                <div class="verdict-score-seg"><div class="verdict-score-seg-fill" style="width:${pct(a.hdrScore)};background:${a.hdrColor}"></div></div>
                <div class="verdict-score-seg"><div class="verdict-score-seg-fill" style="width:${pct(a.audioScore)};background:${a.audioColor}"></div></div>
              </div>
            </div>
            <div>
              <div class="verdict-text" style="text-align:right;font-size:0.68rem;color:#8b949e;max-width:200px">${a.vNote}</div>
            </div>
          </div>
        </div>`;
    }

    function formatBytes(bytes) {
        if (!bytes) return "?";
        let num = parseInt(bytes);
        if (num > 1024 ** 3) return (num / (1024 ** 3)).toFixed(2) + " GB";
        if (num > 1024 ** 2) return (num / (1024 ** 2)).toFixed(2) + " MB";
        return (num / 1024).toFixed(2) + " KB";
    }

    function fmtRaw(info, tool) {
        let out = "";
        if (tool === 'mediainfo' && info.media && info.media.track) {
            info.media.track.forEach(t => {
                out += `[${t['@type']}]\n`;
                if (t['@type'] === 'General') {
                    out += `Format     : ${t.Format || '?'}\n`;
                    out += `Size       : ${formatBytes(t.FileSize)}\n`;
                    out += `Duration   : ${parseFloat(t.Duration || 0).toFixed(2)}s\n`;
                    if (t.OverallBitRate) out += `Bitrate    : ${formatBytes(t.OverallBitRate)}/s\n`;
                } else if (t['@type'] === 'Video') {
                    out += `Codec      : ${t.Format || '?'}\n`;
                    out += `Resolution : ${t.Width}x${t.Height} @ ${t.FrameRate}fps\n`;
                    out += `Bit depth  : ${t.BitDepth || '?'}-bit\n`;
                    if (t.BitRate) out += `Bitrate    : ${formatBytes(t.BitRate)}/s\n`;
                    if (t.HDR_Format) out += `HDR        : ${t.HDR_Format}\n`;
                } else if (t['@type'] === 'Audio') {
                    const lang = (t.Language || 'und').toUpperCase();
                    out += `Language   : ${lang}\n`;
                    out += `Format     : ${t.Format || '?'}\n`;
                    out += `Channels   : ${t.Channels || t['Channel(s)'] || '?'}ch\n`;
                }
                out += '\n';
            });
        } else if (tool === 'ffprobe' && info.streams) {
            info.streams.forEach(s => {
                out += `[${s.codec_type.toUpperCase()}]\n`;
                out += `Codec      : ${s.codec_name} (${s.codec_long_name})\n`;
                if (s.codec_type === 'video') {
                    out += `Resolution : ${s.width}x${s.height}\n`;
                    if (s.bit_rate) out += `Bitrate    : ${formatBytes(s.bit_rate)}/s\n`;
                    if (s.color_transfer) out += `Color Trns : ${s.color_transfer}\n`;
                } else if (s.codec_type === 'audio') {
                    out += `Channels   : ${s.channels}\n`;
                    let lang = info.tags && info.tags.language ? info.tags.language : '';
                    if (lang) out += `Language   : ${lang}\n`;
                    if (s.bit_rate) out += `Bitrate    : ${formatBytes(s.bit_rate)}/s\n`;
                }
                out += '\n';
            });
        } else {
            out = JSON.stringify(info, null, 2);
        }
        return out;
    }
});
