package com.zju.easyconnect.suite

// A short-lived tunnel is still a failed attempt; avoid rapid reconnect loops.
internal fun nextRetryAttempt(previous: Int, connectedMs: Long?): Int =
    if (connectedMs != null && connectedMs >= 60_000L) 1 else minOf(previous + 1, 8)

internal fun retryDelay(attempt: Int): Int = minOf(3 * (1 shl minOf(attempt - 1, 7)), 300)
