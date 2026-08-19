export const formatBytes = n => {
    n = Number(n || 0);
    // 负数或 NaN 一律按 0 处理，避免输出 "-0.00 B" 或异常单位
    if (!(n > 0)) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let i = Math.floor(Math.log(n) / Math.log(1024));
    if (!Number.isFinite(i) || i < 0) i = 0;
    if (i >= u.length) i = u.length - 1;
    return (n / Math.pow(1024, i)).toFixed(2) + ' ' + u[i];
};

export const formatBytesParts = n => {
    n = Number(n || 0);
    if (!(n > 0)) return { value: '0', unit: 'B' };
    const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let i = Math.floor(Math.log(n) / Math.log(1024));
    if (!Number.isFinite(i) || i < 0) i = 0;
    if (i >= u.length) i = u.length - 1;
    return {
        value: (n / Math.pow(1024, i)).toFixed(2),
        unit: u[i],
    };
};

export const formatSpeed = n => formatBytes(n) + '/s';

export const formatSpeedParts = n => {
    const parts = formatBytesParts(n);
    return {
        value: parts.value,
        unit: parts.unit + '/s',
    };
};

export const formatRatio = n => Number(n || 0).toFixed(3);

export const formatTime = s => {
    if (!s) return '未上报';
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return '未知';
    return d.toLocaleString('zh-CN', { hour12: false });
};

export const formatRelative = s => {
    if (!s) return '暂无';
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return '未知';
    const diff = d.getTime() - Date.now();
    const abs = Math.abs(diff);
    const sec = Math.floor(abs / 1000);
    let text = '';
    if (sec < 60) {
        text = sec + ' 秒';
    } else if (sec < 3600) {
        text = Math.floor(sec / 60) + ' 分钟';
    } else if (sec < 86400) {
        text = Math.floor(sec / 3600) + ' 小时';
    } else {
        text = Math.floor(sec / 86400) + ' 天';
    }
    return diff >= 0 ? text + '后' : text + '前';
};

export const formatDuration = seconds => {
    seconds = Number(seconds || 0);
    if (seconds <= 0) return '-';
    if (seconds < 60) return seconds + ' 秒';
    if (seconds < 3600) {
        const min = Math.floor(seconds / 60);
        const sec = seconds % 60;
        return sec ? min + ' 分 ' + sec + ' 秒' : min + ' 分';
    }
    if (seconds < 86400) {
        const hour = Math.floor(seconds / 3600);
        const min = Math.floor((seconds % 3600) / 60);
        return min ? hour + ' 小时 ' + min + ' 分' : hour + ' 小时';
    }
    const day = Math.floor(seconds / 86400);
    const hour = Math.floor((seconds % 86400) / 3600);
    return hour ? day + ' 天 ' + hour + ' 小时' : day + ' 天';
};

export const eventLabel = event => {
    if (event === 'started') return '启动上报';
    if (event === 'stopped') return '停止上报';
    if (event === 'completed') return '完成上报';
    return '常规上报';
};
