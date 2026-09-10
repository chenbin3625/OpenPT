// 最小可订阅状态容器：set 做浅合并并通知订阅者。
export function createStore(initial) {
    let state = { ...initial };
    const subscribers = new Set();

    return {
        get: () => state,
        set(patch) {
            state = { ...state, ...patch };
            for (const fn of subscribers) fn(state);
        },
        subscribe(fn) {
            subscribers.add(fn);
            fn(state);
            return () => subscribers.delete(fn);
        },
    };
}
