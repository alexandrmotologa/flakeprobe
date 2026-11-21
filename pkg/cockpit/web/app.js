let reportData = null;
let selectedNodeId = null;

async function initCockpit() {
  try {
    const res = await fetch('/api/report');
    if (!res.ok) throw new Error('Failed to load report');
    reportData = await res.json();
    renderAll();
    setupSSE();
  } catch (err) {
    console.error('Initialization error:', err);
    document.getElementById('status-badge').textContent = 'OFFLINE';
    document.getElementById('status-badge').className = 'status-badge status-fail';
  }
}

function renderAll() {
  if (!reportData || !reportData.analysis) return;
  const analysis = reportData.analysis;

  // Header
  document.getElementById('target-desc').textContent = reportData.target_description || 'Target';
  const badge = document.getElementById('status-badge');
  if (analysis.is_unstable) {
    badge.textContent = 'FAIL (UNSTABLE)';
    badge.className = 'status-badge status-fail';
  } else if (analysis.fri > 0.05) {
    badge.textContent = 'WARN (JITTER)';
    badge.className = 'status-badge status-warn';
  } else {
    badge.textContent = 'PASS (STABLE)';
    badge.className = 'status-badge status-pass';
  }

  // Metrics
  document.getElementById('metric-fri').textContent = analysis.fri.toFixed(4);
  document.getElementById('metric-entropy').textContent = (analysis.entropy_profile.max_node_entropy || 0).toFixed(3) + ' bits';
  document.getElementById('metric-distance').textContent = (analysis.distance_profile.mean_normalized_distance || 0).toFixed(4);
  document.getElementById('metric-success').textContent = ((analysis.success_rate || 0) * 100).toFixed(1) + '%';
  
  if (analysis.cost_profile) {
    document.getElementById('metric-cost-jitter').textContent = (analysis.cost_profile.cost_jitter_ratio || 1.0).toFixed(2) + 'x';
  } else {
    document.getElementById('metric-cost-jitter').textContent = '1.00x';
  }

  // Bifurcations
  renderBifurcations(analysis.entropy_profile.bifurcation_nodes || []);

  // Runs
  renderRuns(reportData.runs || []);

  // Populate Diff Selectors
  populateDiffSelectors(reportData.runs || []);

  // Mitigations
  loadMitigations();

  // DAG
  if (analysis.state_dag) {
    renderDAG(analysis.state_dag, analysis.entropy_profile);
  }
}

function populateDiffSelectors(runs) {
  const selA = document.getElementById('diff-select-a');
  const selB = document.getElementById('diff-select-b');
  if (!selA || !selB) return;

  selA.innerHTML = '';
  selB.innerHTML = '';

  runs.forEach((r, idx) => {
    const optA = document.createElement('option');
    optA.value = r.id;
    optA.textContent = `${r.id} (${r.success ? 'Pass' : 'Fail'}, ${r.steps ? r.steps.length : 0} steps)`;
    selA.appendChild(optA);

    const optB = document.createElement('option');
    optB.value = r.id;
    optB.textContent = `${r.id} (${r.success ? 'Pass' : 'Fail'}, ${r.steps ? r.steps.length : 0} steps)`;
    if (idx === 1 || (idx === runs.length - 1 && runs.length > 1)) {
      optB.selected = true;
    }
    selB.appendChild(optB);
  });
}

async function triggerRunDiff() {
  const runA = document.getElementById('diff-select-a').value;
  const runB = document.getElementById('diff-select-b').value;
  const container = document.getElementById('diff-results-container');
  if (!runA || !runB) return;

  container.innerHTML = '<p style="color: var(--text-muted);">Comparing trajectories...</p>';
  try {
    const res = await fetch(`/api/diff?runA=${encodeURIComponent(runA)}&runB=${encodeURIComponent(runB)}`);
    if (!res.ok) throw new Error('Failed to fetch diff');
    const diff = await res.json();

    let html = `
      <div style="margin-bottom: 12px; font-size: 11px; padding: 8px; background: var(--bg-primary); border-radius: 4px; border: 1px solid var(--border-color);">
        <div style="font-weight: 700; color: ${diff.are_identical ? 'var(--accent-green)' : 'var(--accent-red)'}; margin-bottom: 4px;">
          ${diff.are_identical ? 'IDENTICAL PATHS' : `DIVERGES AT STEP ${diff.divergence_step}`}
        </div>
        <div style="color: var(--text-secondary); font-family: var(--font-mono);">
          Token Delta: ${diff.token_delta > 0 ? '+' : ''}${diff.token_delta} | Duration: ${diff.duration_delta_ms}ms
        </div>
      </div>
    `;

    (diff.steps || []).forEach(s => {
      let chipClass = 'diff-match';
      if (s.status === 'TOOL_SUBSTITUTION' || s.status === 'INSERTION' || s.status === 'DELETION') {
        chipClass = 'diff-sub';
      } else if (s.status === 'ARG_DIFF') {
        chipClass = 'diff-arg';
      }

      const labelA = s.step_a ? (s.step_a.tool_name || s.step_a.action_type) : '<empty>';
      const labelB = s.step_b ? (s.step_b.tool_name || s.step_b.action_type) : '<empty>';

      html += `
        <div class="diff-step-card">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
            <span style="font-family: var(--font-mono); font-weight: 700;">Step ${s.index}</span>
            <span class="diff-chip ${chipClass}">${s.status}</span>
          </div>
          <div style="font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary);">
            <div>${diff.run_a_id}: <span style="color: var(--text-primary); font-weight: 600;">${escapeHtml(labelA)}</span></div>
            <div>${diff.run_b_id}: <span style="color: var(--text-primary); font-weight: 600;">${escapeHtml(labelB)}</span></div>
          </div>
          ${(s.arg_diffs || []).map(ad => `
            <div style="margin-top: 4px; padding: 4px; background: var(--bg-tertiary); border-radius: 3px; font-size: 10px; font-family: var(--font-mono); color: var(--accent-yellow);">
              ${escapeHtml(ad.field)}: [${escapeHtml(ad.val_a)}] &ne; [${escapeHtml(ad.val_b)}]
            </div>
          `).join('')}
        </div>
      `;
    });

    container.innerHTML = html;
  } catch (err) {
    container.innerHTML = `<p style="color: var(--accent-red); font-size: 12px;">Diff error: ${escapeHtml(err.message)}</p>`;
  }
}

async function loadMitigations() {
  const container = document.getElementById('mitigations-container');
  if (!container) return;

  try {
    const res = await fetch('/api/suggest');
    if (!res.ok) throw new Error('Failed to load mitigations');
    const data = await res.json();

    if (!data.directives || data.directives.length === 0) {
      container.innerHTML = '<p style="color: var(--accent-green);">No mitigations required. Agent decision boundaries are stable.</p>';
      return;
    }

    let html = '';
    data.directives.forEach((d, idx) => {
      html += `
        <div class="mitigation-card">
          <div style="display: flex; justify-content: space-between; margin-bottom: 4px;">
            <span style="font-weight: 700; font-size: 12px;">[${idx + 1}] Step ${d.step_index}</span>
            <span class="diff-chip ${d.priority === 'CRITICAL' ? 'diff-sub' : 'diff-arg'}">${d.priority}</span>
          </div>
          <div style="font-size: 11px; color: var(--text-secondary); margin-bottom: 6px;">
            ${escapeHtml(d.diagnostic_issue)}
          </div>
          <div style="background: var(--bg-tertiary); padding: 6px 8px; border-radius: 4px; font-family: var(--font-mono); font-size: 10px; color: var(--accent-green);">
            ${escapeHtml(d.suggested_directive)}
          </div>
        </div>
      `;
    });

    html += `
      <div class="section-title" style="margin-top: 16px;">System Prompt Patch</div>
      <pre>${escapeHtml(data.system_prompt_patch_markdown || '')}</pre>
    `;

    container.innerHTML = html;
  } catch (err) {
    container.innerHTML = `<p style="color: var(--text-muted); font-size: 12px;">Mitigations unavailable.</p>`;
  }
}

function openRunModal() {
  document.getElementById('run-modal').style.display = 'flex';
}

function closeRunModal() {
  document.getElementById('run-modal').style.display = 'none';
}

async function submitRunBenchmark() {
  const archetype = document.getElementById('modal-archetype').value;
  const runs = parseInt(document.getElementById('modal-runs').value, 10) || 10;
  const maxFRI = parseFloat(document.getElementById('modal-max-fri').value) || 0.20;

  closeRunModal();
  document.getElementById('status-badge').textContent = 'BENCHMARK RUNNING';
  document.getElementById('status-badge').className = 'status-badge status-warn';

  try {
    const res = await fetch('/api/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        target_type: 'mock',
        mock_archetype: archetype,
        runs: runs,
        concurrency: 4,
        max_fri: maxFRI,
        prompt: `Benchmark run (${archetype})`,
      }),
    });
    if (!res.ok) throw new Error('Benchmark start failed');
  } catch (err) {
    alert('Failed to launch benchmark: ' + err.message);
  }
}

function renderBifurcations(nodes) {
  const container = document.getElementById('bifurcation-list');
  if (nodes.length === 0) {
    container.innerHTML = '<p style="color: var(--accent-green);">No decision bifurcations found. Agent is deterministic.</p>';
    return;
  }

  let html = '';
  nodes.forEach(bp => {
    html += `
      <div class="bifurcation-card" onclick="selectNode('${bp.node_id}')">
        <div style="font-weight: 600; color: var(--text-primary);">Step ${bp.step_index}: ${escapeHtml(bp.label)}</div>
        <div style="font-family: var(--font-mono); font-size: 11px; color: var(--accent-yellow); margin-top: 2px;">
          Entropy: ${bp.entropy.toFixed(3)} bits (${(bp.normalized * 100).toFixed(0)}% max)
        </div>
        <div style="margin-top: 6px; font-size: 11px;">
          ${bp.branch_options.map(b => `
            <div style="display: flex; justify-content: space-between; color: var(--text-secondary);">
              <span>&rarr; ${escapeHtml(b.target_node_id)}</span>
              <span style="font-family: var(--font-mono);">${(b.probability * 100).toFixed(0)}% (${b.count})</span>
            </div>
          `).join('')}
        </div>
      </div>
    `;
  });
  container.innerHTML = html;
}

function renderRuns(runs) {
  const container = document.getElementById('runs-list');
  if (runs.length === 0) {
    container.innerHTML = '<p style="color: var(--text-muted);">No runs recorded.</p>';
    return;
  }

  let html = '';
  runs.forEach(r => {
    html += `
      <div class="run-item" onclick="highlightRun('${r.id}')">
        <div class="run-header">
          <span class="run-id" style="color: ${r.success ? 'var(--accent-green)' : 'var(--accent-red)'};">${r.id}</span>
          <span class="run-steps-count">${r.duration_ms}ms &bull; ${r.steps ? r.steps.length : 0} steps</span>
        </div>
        <div class="run-trajectory">
          ${(r.steps || []).map(s => `
            <span class="step-chip">${escapeHtml(s.action_type === 'tool_call' ? s.tool_name : s.action_type)}</span>
          `).join('')}
        </div>
      </div>
    `;
  });
  container.innerHTML = html;
}

function renderDAG(dag, entropyProfile) {
  const svg = document.getElementById('dag-svg');
  svg.innerHTML = '';

  const nodes = Object.values(dag.nodes);
  if (nodes.length === 0) return;

  const stepGroups = {};
  nodes.forEach(n => {
    const s = n.step_index;
    if (!stepGroups[s]) stepGroups[s] = [];
    stepGroups[s].push(n);
  });

  const sortedSteps = Object.keys(stepGroups).map(Number).sort((a, b) => a - b);
  const stepSpacing = 240;
  const startX = 60;
  const nodeWidth = 160;
  const nodeHeight = 54;
  const nodePositions = {};

  let maxY = 400;

  sortedSteps.forEach((s, colIdx) => {
    const group = stepGroups[s];
    const colX = startX + colIdx * stepSpacing;
    const startY = 80;
    const verticalGap = 75;

    group.forEach((node, rowIdx) => {
      const colY = startY + rowIdx * verticalGap;
      nodePositions[node.id] = { x: colX, y: colY };
      if (colY + nodeHeight > maxY) maxY = colY + nodeHeight;
    });
  });

  const totalWidth = startX + sortedSteps.length * stepSpacing + 100;
  svg.setAttribute('viewBox', `0 0 ${Math.max(totalWidth, 900)} ${Math.max(maxY + 80, 500)}`);

  const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
  defs.innerHTML = `
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 9 5 L 0 9 z" fill="var(--border-color)" />
    </marker>
    <marker id="arrow-active" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 9 5 L 0 9 z" fill="var(--accent-cyan)" />
    </marker>
  `;
  svg.appendChild(defs);

  (dag.edges || []).forEach(edge => {
    const srcPos = nodePositions[edge.source];
    const tgtPos = nodePositions[edge.target];
    if (!srcPos || !tgtPos) return;

    const x1 = srcPos.x + nodeWidth;
    const y1 = srcPos.y + nodeHeight / 2;
    const x2 = tgtPos.x;
    const y2 = tgtPos.y + nodeHeight / 2;

    const dx = (x2 - x1) / 2;
    const pathD = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;

    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', pathD);
    path.setAttribute('fill', 'none');
    path.setAttribute('stroke', 'var(--border-color)');
    const strokeWidth = Math.max(1.5, edge.probability * 4.5);
    path.setAttribute('stroke-width', strokeWidth.toString());
    path.setAttribute('marker-end', 'url(#arrow)');
    path.classList.add('dag-edge');
    path.dataset.source = edge.source;
    path.dataset.target = edge.target;
    svg.appendChild(path);

    if (edge.probability < 0.99) {
      const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      text.setAttribute('x', ((x1 + x2) / 2).toString());
      text.setAttribute('y', ((y1 + y2) / 2 - 6).toString());
      text.setAttribute('fill', 'var(--text-muted)');
      text.setAttribute('font-size', '10px');
      text.setAttribute('font-family', 'var(--font-mono)');
      text.setAttribute('text-anchor', 'middle');
      text.textContent = `${(edge.probability * 100).toFixed(0)}%`;
      svg.appendChild(text);
    }
  });

  nodes.forEach(node => {
    const pos = nodePositions[node.id];
    if (!pos) return;

    const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    g.setAttribute('transform', `translate(${pos.x}, ${pos.y})`);
    g.style.cursor = 'pointer';
    g.onclick = () => selectNode(node.id);

    let borderColor = 'var(--border-color)';
    let badgeText = `${node.visits} runs`;
    if (node.entropy >= 1.0) {
      borderColor = 'var(--accent-red)';
      badgeText = `H=${node.entropy.toFixed(2)}`;
    } else if (node.entropy > 0.05) {
      borderColor = 'var(--accent-yellow)';
      badgeText = `H=${node.entropy.toFixed(2)}`;
    } else if (node.id === 'START') {
      borderColor = 'var(--accent-cyan)';
    }

    g.innerHTML = `
      <rect width="${nodeWidth}" height="${nodeHeight}" rx="6" fill="var(--bg-secondary)" stroke="${borderColor}" stroke-width="${node.entropy > 0 ? '2' : '1'}" />
      <text x="12" y="24" fill="var(--text-primary)" font-size="12px" font-weight="600" font-family="var(--font-sans)">
        ${escapeHtml(node.label || node.id)}
      </text>
      <text x="12" y="42" fill="var(--text-secondary)" font-size="10px" font-family="var(--font-mono)">
        ${badgeText} &bull; ${(node.avg_duration_ms || 0).toFixed(0)}ms
      </text>
    `;
    svg.appendChild(g);
  });
}

function selectNode(nodeId) {
  selectedNodeId = nodeId;
  switchTab('inspector');

  const dag = reportData.analysis.state_dag;
  const node = dag.nodes[nodeId];
  if (!node) return;

  const container = document.getElementById('node-details');
  container.innerHTML = `
    <div style="font-weight: 700; font-size: 14px; color: var(--text-primary); margin-bottom: 4px;">
      ${escapeHtml(node.label || node.id)}
    </div>
    <div style="font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary); margin-bottom: 10px;">
      ID: ${escapeHtml(node.id)}
    </div>
    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-bottom: 12px;">
      <div style="background: var(--bg-tertiary); padding: 8px; border-radius: 4px;">
        <div style="font-size: 10px; color: var(--text-muted); text-transform: uppercase;">Shannon Entropy</div>
        <div style="font-family: var(--font-mono); font-size: 14px; font-weight: 700; color: ${node.entropy > 0 ? 'var(--accent-yellow)' : 'var(--accent-green)'};">
          ${(node.entropy || 0).toFixed(3)} bits
        </div>
      </div>
      <div style="background: var(--bg-tertiary); padding: 8px; border-radius: 4px;">
        <div style="font-size: 10px; color: var(--text-muted); text-transform: uppercase;">Visitations</div>
        <div style="font-family: var(--font-mono); font-size: 14px; font-weight: 700;">
          ${node.visits} / ${dag.num_runs}
        </div>
      </div>
    </div>
    <div class="section-title">Outgoing Transitions</div>
    <div style="margin-bottom: 12px; font-size: 11px;">
      ${(node.outgoing_edges || []).map(e => `
        <div style="display: flex; justify-content: space-between; margin-bottom: 4px; padding: 4px 6px; background: var(--bg-tertiary); border-radius: 4px;">
          <span>&rarr; ${escapeHtml(e.target)}</span>
          <span style="font-family: var(--font-mono); font-weight: 600;">${(e.probability * 100).toFixed(0)}%</span>
        </div>
      `).join('') || '<span style="color: var(--text-muted);">Terminal node (no successors)</span>'}
    </div>
    ${node.sample_args ? `
      <div class="section-title">Sample Arguments</div>
      <pre>${escapeHtml(JSON.stringify(node.sample_args, null, 2))}</pre>
    ` : ''}
  `;
}

function highlightRun(runId) {
  const run = (reportData.runs || []).find(r => r.id === runId);
  if (!run) return;

  const edges = document.querySelectorAll('.dag-edge');
  edges.forEach(e => {
    e.setAttribute('stroke', 'var(--border-color)');
    e.setAttribute('marker-end', 'url(#arrow)');
  });

  let prev = 'START';
  (run.steps || []).forEach((st, idx) => {
    let curr = `step_${idx}:${st.tool_name}`;
    if (st.is_terminal) curr = `step_${idx}:TERMINAL`;

    edges.forEach(e => {
      if (e.dataset.source === prev && e.dataset.target === curr) {
        e.setAttribute('stroke', 'var(--accent-cyan)');
        e.setAttribute('marker-end', 'url(#arrow-active)');
      }
    });
    prev = curr;
  });
}

function switchTab(tabName) {
  const tabs = ['inspector', 'bifurcations', 'diff', 'mitigations', 'runs'];
  document.querySelectorAll('.tab-btn').forEach((b, i) => {
    if (tabs[i] === tabName) {
      b.classList.add('active');
    } else {
      b.classList.remove('active');
    }
  });

  document.querySelectorAll('.sidebar-content').forEach(c => c.style.display = 'none');
  const target = document.getElementById(`tab-${tabName}`);
  if (target) target.style.display = 'block';

  if (tabName === 'diff') {
    triggerRunDiff();
  }
}

function setupSSE() {
  const sse = new EventSource('/api/stream');
  sse.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'report_update') {
        reportData = data.payload;
        renderAll();
      }
    } catch (e) {}
  };
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

window.addEventListener('DOMContentLoaded', initCockpit);
