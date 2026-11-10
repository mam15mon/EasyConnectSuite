package com.zju.easyconnect.suite

import java.util.concurrent.ScheduledThreadPoolExecutor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

/** Records bounded Java thread evidence while a session operation is blocked. */
internal class WaitEvidence private constructor(
    private val request: Int,
    private val stage: String,
    private val cancelled: () -> Boolean,
    private val write: (String) -> Unit,
    private val thresholdNanos: Long,
    private val rateLimitNanos: Long
) {
    private val id = operationIds.incrementAndGet()
    private val caller = Thread.currentThread().id
    private val started = System.nanoTime()
    private var finished = false
    private var reported = false
    private var future: java.util.concurrent.ScheduledFuture<*>? = null

    companion object {
        private val operationIds = AtomicLong()
        private val snapshotIds = AtomicLong()
        private var lastSnapshotAt = 0L
        private var lastSnapshotId = 0L
        private val scheduler = ScheduledThreadPoolExecutor(1) { task ->
            Thread(task, "vpn-wait-observer").apply { isDaemon = true }
        }.apply { removeOnCancelPolicy = true }

        fun begin(request: Int, stage: String, cancelled: () -> Boolean, write: (String) -> Unit,
                  intervalMs: Long = 5000, thresholdMs: Long = 30000, rateLimitMs: Long = 60000): WaitEvidence {
            val operation = WaitEvidence(request, stage, cancelled, write,
                TimeUnit.MILLISECONDS.toNanos(thresholdMs), TimeUnit.MILLISECONDS.toNanos(rateLimitMs))
            operation.write(operation.line("phase=begin"))
            operation.future = scheduler.scheduleAtFixedRate({ operation.check() }, intervalMs, intervalMs, TimeUnit.MILLISECONDS)
            return operation
        }

        @Synchronized private fun snapshot(now: Long, rateLimit: Long): Pair<Long, Boolean> {
            if (lastSnapshotAt != 0L && now - lastSnapshotAt < rateLimit) return lastSnapshotId to false
            lastSnapshotAt = now
            lastSnapshotId = snapshotIds.incrementAndGet()
            return lastSnapshotId to true
        }

        private fun safeName(value: String): String = value.replace(Regex("[^A-Za-z0-9_.$-]"), "_")
    }

    private fun line(detail: String): String =
        "android_wait request=$request operation=$id thread=$caller stage=$stage elapsed_ms=${TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started)} cancelled=${cancelled()} $detail"

    @Synchronized private fun check() {
        if (finished || reported || System.nanoTime() - started < thresholdNanos) return
        reported = true
        val (snapshotId, capture) = snapshot(System.nanoTime(), rateLimitNanos)
        write(line("phase=pending evidence=no_stage_progress root_cause=undetermined snapshot=$snapshotId captured=$capture"))
        if (!capture) return
        val all = Thread.getAllStackTraces().entries.sortedBy { it.key.id }
        var chars = 0
        var truncated = all.size > 64
        for ((thread, frames) in all.take(64)) {
            val header = line("snapshot=$snapshotId thread_id=${thread.id} state=${thread.state}")
            if (chars + header.length > 180000) { truncated = true; break }
            write(header)
            chars += header.length
            if (frames.size > 32) truncated = true
            for (frame in frames.take(32)) {
                val text = line("snapshot=$snapshotId thread_id=${thread.id} frame=${safeName(frame.className)}.${safeName(frame.methodName)} source=${safeName(frame.fileName ?: "unknown")}:${frame.lineNumber}")
                if (chars + text.length > 180000) { truncated = true; break }
                write(text)
                chars += text.length
            }
            if (chars >= 180000) break
        }
        write(line("phase=snapshot_end snapshot=$snapshotId truncated=$truncated chars=$chars"))
    }

    @Synchronized fun finish(error: Throwable? = null) {
        if (finished) return
        finished = true
        future?.cancel(false)
        val outcome = when {
            error is InterruptedException && cancelled() -> "cancelled"
            error == null -> "success"
            else -> "failed"
        }
        val type = error?.javaClass?.simpleName ?: "none"
        write(line("phase=end outcome=$outcome timeout=false error_type=$type"))
    }
}
