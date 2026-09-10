// 单行种子渲染。行元素按 info_hash 复用：SSE 每次推送只更新文本与状态类，
// 不重建节点，这样悬浮卡片、tooltip 与滚动位置都不会被打断。
import { h, clearNode } from '../lib/dom.js';
import {
    formatBytes, formatSpeed, formatRatio, formatTime,
    formatRelative, formatDuration, eventLabel,
} from '../lib/format.js';
import { copyText } from '../lib/clipboard.js';
import { icon } from './icons.js';
import { showToast } from './toast.js';
import { attachPopover } from './popover.js';

function trackerOrdinal(t) {
    return t.tracker_count > 0 ? `${(t.tracker_index || 0) + 1}/${t.tracker_count}` : '-';
}

function statusMeta(t) {
    if (t.has_issue) {
        if (t.failures > 0) return { tone: 'danger', text: `失败 ${t.failures}`, icon: 'fail' };
        return { tone: 'warning', text: '异常', icon: 'alert' };
    }
    return { tone: 'success', text: '正常', icon: 'ok' };
}

function speedTone(bps) {
    if (!(bps > 0)) return 'muted';
    if (bps < 10240) return 'default';
    if (bps < 102400) return 'warning';
    return 'success';
}

function ratioTone(r) {
    if (r >= 1) return 'success';
    if (r >= 0.5) return 'warning';
    return 'danger';
}

// 分享率进度：达标（>=1）时以 2.0 为满格，未达标时直接按百分比铺满
function ratioPercent(r) {
    const ratio = Number(r || 0);
    if (ratio >= 1) return Math.min(100, (ratio / 2) * 100);
    return Math.max(0, Math.min(100, ratio * 100));
}

function detailRow(label, value) {
    return h('div', { class: 'detail-row' },
        h('span', { class: 'detail-label', text: label }),
        h('span', { class: 'detail-value' }, value),
    );
}

/** 悬浮卡片内容：异常种子的完整上报状态与最近错误。 */
function buildStatusDetail(t) {
    const meta = statusMeta(t);
    const box = h('div', { class: 'detail' },
        detailRow('运行状态', h('span', { class: `pill tone-${meta.tone}` },
            icon(meta.icon, { size: 13 }), h('span', { text: meta.text }))),
        detailRow('上次上报', `${formatTime(t.last_announce_at)}（${formatRelative(t.last_announce_at)}）`),
        detailRow('下次上报', `${formatTime(t.next_announce_at)}（${formatRelative(t.next_announce_at)}）`),
        detailRow('上报类型', `${eventLabel(t.next_event)} · 周期 ${formatDuration(t.last_interval_seconds)}`),
        detailRow('Tracker 节点', h('span', { class: 'detail-inline' },
            icon('tracker', { size: 13, class: 'tone-primary' }),
            h('code', { text: t.tracker_host || '-' }),
            h('span', { class: 'tag', text: `序号 ${trackerOrdinal(t)}` }),
        )),
        detailRow('Peers 节点', h('span', { class: 'detail-inline' },
            h('span', null, '做种 ', h('b', { class: 'tone-success', text: String(t.seeders || 0) })),
            h('span', null, '下载 ', h('b', {
                class: t.leechers > 0 ? 'tone-warning' : 'tone-muted',
                text: String(t.leechers || 0),
            })),
        )),
    );

    if (t.issue_reason) {
        box.appendChild(detailRow('异常原因', h('span', { class: 'tone-warning', text: t.issue_reason })));
    }

    if (t.last_error) {
        const copyBtn = h('button', {
            class: 'icon-btn icon-btn-sm',
            type: 'button',
            'data-tip': '复制错误信息',
            'aria-label': '复制错误信息',
            onClick: async () => {
                const ok = await copyText(t.last_error);
                showToast(ok ? '错误信息已复制到剪贴板' : '复制失败，请手动选择文本', ok ? 'success' : 'error');
            },
        }, icon('copy', { size: 13 }));

        box.appendChild(h('div', { class: 'error-box' },
            h('div', { class: 'error-head' },
                h('span', { class: 'error-title', text: '最新错误详情' }),
                copyBtn,
            ),
            h('code', { class: 'error-text', text: t.last_error }),
        ));
    }

    return box;
}

/**
 * 创建一行，并返回可复用的更新句柄。
 * @param {object} torrent
 */
export function createTorrentRow(torrent) {
    let current = torrent;

    const nameMain = h('span', { class: 'name-main' });
    const nameTag = h('span', { class: 'tag' });
    const nameEvent = h('span', { class: 'muted', text: '' });

    const pill = h('span', { class: 'pill', tabindex: '0' });
    const statusSub = h('span', { class: 'sub muted' });

    const speedBolt = icon('bolt', { size: 11, fill: true, class: 'bolt' });
    const speedText = h('span');
    const speedWrap = h('span', { class: 'speed' }, speedBolt, speedText);

    const uploaded = h('span', { class: 'num-text' });
    const size = h('span', { class: 'num-text muted' });

    const peersTotal = h('b', { class: 'num-text' });
    const peersSub = h('span', { class: 'sub muted' });

    const ratioFill = h('i');
    const ratioValue = h('b', { class: 'num-text' });

    const nextRel = h('b', { class: 'num-text' });
    const nextAbs = h('span', { class: 'sub muted' });

    const trackerHost = h('span', { class: 'host' });
    const trackerCount = h('span', { class: 'sub muted' });

    const hashText = h('code', { class: 'hash-text' });
    const hashCopy = h('button', {
        class: 'icon-btn icon-btn-sm copy-btn',
        type: 'button',
        'data-tip': '复制 InfoHash',
        'aria-label': '复制 InfoHash',
        onClick: async () => {
            const ok = await copyText(current.info_hash);
            showToast(ok ? 'InfoHash 已复制到剪贴板' : '复制失败，请手动选择文本', ok ? 'success' : 'error');
        },
    }, icon('copy', { size: 13 }));

    const tr = h('tr', { class: 'row' },
        h('td', { class: 'col-name' },
            h('div', { class: 'name' },
                nameMain,
                h('span', { class: 'name-sub' }, nameTag, nameEvent),
            ),
        ),
        h('td', { class: 'col-status num' }, h('div', { class: 'stack-end' }, pill, statusSub)),
        h('td', { class: 'col-speed num' }, speedWrap),
        h('td', { class: 'col-num num' }, uploaded),
        h('td', { class: 'col-num num' }, size),
        h('td', { class: 'col-peers num' }, h('div', { class: 'stack-end' }, peersTotal, peersSub)),
        h('td', { class: 'col-ratio num' },
            h('div', { class: 'ratio' }, h('span', { class: 'ratio-bar' }, ratioFill), ratioValue),
        ),
        h('td', { class: 'col-time num' }, h('div', { class: 'stack-end' }, nextRel, nextAbs)),
        h('td', { class: 'col-tracker' }, h('div', { class: 'stack' }, trackerHost, trackerCount)),
        h('td', { class: 'col-hash' }, h('div', { class: 'hash' }, hashText, hashCopy)),
    );

    // 悬浮详情只对异常种子开放，与正常行的静态标签保持一致的交互预期
    attachPopover(pill, () => buildStatusDetail(current), {
        title: () => current.name || '',
        canOpen: () => Boolean(current.has_issue),
    });

    function renderTimes() {
        const t = current;
        statusSub.textContent = t.failures > 0
            ? `重试 ${formatRelative(t.next_announce_at)}`
            : eventLabel(t.next_event);
        nextRel.textContent = formatRelative(t.next_announce_at);
        nextAbs.textContent = formatTime(t.next_announce_at);
    }

    function update(t) {
        current = t;
        tr.classList.toggle('is-issue', Boolean(t.has_issue));

        nameMain.textContent = t.name || '-';
        nameMain.dataset.tip = t.name || '';
        nameTag.textContent = `Tracker ${trackerOrdinal(t)}`;
        nameEvent.textContent = eventLabel(t.next_event);

        const meta = statusMeta(t);
        pill.className = `pill tone-${meta.tone}${t.has_issue ? ' is-interactive' : ''}`;
        clearNode(pill);
        pill.appendChild(icon(meta.icon, { size: 13 }));
        pill.appendChild(h('span', { text: meta.text }));
        pill.setAttribute('aria-label', t.has_issue && t.issue_reason ? `${meta.text}：${t.issue_reason}` : meta.text);

        speedWrap.className = `speed tone-${speedTone(t.speed_bps || 0)}`;
        speedBolt.style.display = (t.speed_bps || 0) > 0 ? '' : 'none';
        speedText.textContent = formatSpeed(t.speed_bps);

        uploaded.textContent = formatBytes(t.uploaded);
        size.textContent = formatBytes(t.size);

        peersTotal.textContent = String((t.seeders || 0) + (t.leechers || 0));
        peersSub.textContent = `做种 ${t.seeders || 0} / 下 ${t.leechers || 0}`;

        const tone = ratioTone(Number(t.ratio || 0));
        ratioFill.className = `tone-${tone}`;
        ratioFill.style.width = `${ratioPercent(t.ratio)}%`;
        ratioValue.className = `num-text tone-${tone}`;
        ratioValue.textContent = formatRatio(t.ratio);

        trackerHost.textContent = t.tracker_host || '-';
        trackerHost.dataset.tip = t.tracker_host || '';
        trackerCount.textContent = `共 ${t.tracker_count || 1} 个节点`;

        hashText.textContent = t.info_hash ? `${t.info_hash.slice(0, 10)}…` : '-';
        hashCopy.style.display = t.info_hash ? '' : 'none';

        renderTimes();
    }

    update(torrent);
    return { tr, key: torrent.info_hash, update, refreshTimes: renderTimes };
}
