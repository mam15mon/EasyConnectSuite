package com.zju.easyconnect.suite

import java.util.concurrent.CountDownLatch

/** Coordinates cancellation and cleanup of one owner of the process-wide client. */
internal class SessionCleanup(private val cleanup: () -> Unit) {
    private var cancelled = false
    private var owner = false
    private var cleanupDone: CountDownLatch? = null
    private var failure: Throwable? = null

    @Synchronized fun acquire(): Boolean {
        if (cancelled) return false
        owner = true
        return true
    }

    @Synchronized fun cancel() {
        cancelled = true
        if (owner) startCleanup()
    }

    @Synchronized private fun startCleanup(): CountDownLatch {
        cleanupDone?.let { return it }
        val done = CountDownLatch(1)
        cleanupDone = done
        Thread {
            try {
                cleanup()
            } catch (error: Throwable) {
                synchronized(this) { failure = error }
            } finally {
                done.countDown()
            }
        }.start()
        return done
    }

    fun finish() {
        val done = synchronized(this) {
            cancelled = true
            if (owner) startCleanup() else null
        }
        var interrupted = false
        while (done != null) {
            try {
                done.await()
                break
            } catch (_: InterruptedException) {
                interrupted = true
            }
        }
        if (interrupted) Thread.currentThread().interrupt()
        synchronized(this) { failure?.let { throw it } }
    }
}
