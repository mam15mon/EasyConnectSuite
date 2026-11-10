package com.zju.easyconnect.suite

import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicBoolean

/** Orders process-wide session cleanup before a successor can use Mobile. */
internal class SessionHandoff {
    internal class Session internal constructor(internal val predecessor: Session?) {
        internal val done = CountDownLatch(1)
        private val cancelled = AtomicBoolean(false)
        fun cancel() { cancelled.set(true) }
        fun isCancelled(): Boolean = cancelled.get()
        fun awaitPredecessor(): Boolean {
            predecessor?.done?.await()
            return !isCancelled()
        }
        fun finish() { done.countDown() }
    }

    private var latest: Session? = null

    @Synchronized fun next(): Session = Session(latest).also { latest = it }
}
