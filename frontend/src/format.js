export const formatBytes = n => {
    n = Number(n || 0);
    if (n === 0) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(n) / Math.log(1024));
    return (n / Math.pow(1024, i)).toFixed(2) + ' ' + u[i];
};

export const formatSpeed = n => formatBytes(n) + '/s';

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
    const sec = Math.round(abs / 1000);
    const min = Math.round(sec / 60);
    const hour = Math.round(min / 60);
    const text = hour >= 1 ? hour + ' 小时' : min >= 1 ? min + ' 分钟' : sec + ' 秒';
    return diff >= 0 ? text + '后' : text + '前';
};

export const formatDuration = seconds => {
    seconds = Number(seconds || 0);
    if (seconds <= 0) return '-';
    if (seconds < 60) return seconds + ' 秒';
    const min = Math.floor(seconds / 60);
    const sec = seconds % 60;
    if (min < 60) return sec ? min + ' 分 ' + sec + ' 秒' : min + ' 分';
    const hour = Math.floor(min / 60);
    const rest = min % 60;
    return rest ? hour + ' 小时 ' + rest + ' 分' : hour + ' 小时';
};

export const eventLabel = event => {
    if (event === 'started') return '启动上报';
    if (event === 'stopped') return '停止上报';
    if (event === 'completed') return '完成上报';
    return '常规上报';
};
