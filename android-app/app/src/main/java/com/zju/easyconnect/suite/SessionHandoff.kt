package com.zju.easyconnect.suite

import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicBoolean

/** Orders process-wide session cleanup before a successor can use Mobile. */
internal class SessionHandoff {
    internal class Session internal constructor(predecessor: Session?) {
        @Volatile internal var predecessor: Session? = predecessor
            private set
        internal val done = CountDownLatch(1)
        private val cancelled = AtomicBoolean(false)
        fun cancel() { cancelled.set(true) }
        fun isCancelled(): Boolean = cancelled.get()
        fun awaitPredecessor(): Boolean {
            predecessor?.done?.await()
            return !isCancelled()
        }
        fun finish() {
            // A cancelled intermediate session must not let its successor
            // overtake an older session still performing cleanup.
            var interrupted = false
            while (true) {
                try {
                    predecessor?.done?.await()
                    break
                } catch (_: InterruptedException) {
                    interrupted = true
                }
            }
            predecessor = null
            done.countDown()
            if (interrupted) Thread.currentThread().interrupt()
        }
    }

    private var latest: Session? = null

    @Synchronized fun next(): Session = Session(latest).also { latest = it }
}
