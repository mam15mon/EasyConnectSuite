package com.zju.easyconnect.suite

import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.*
import org.junit.Test

class SessionCleanupTest {
    @Test fun cancellationReturnsWhileCleanupBlocksAndFinishWaits() {
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val finished = CountDownLatch(1)
        val calls = AtomicInteger()
        val session = SessionCleanup {
            calls.incrementAndGet()
            entered.countDown()
            release.await()
        }
        assertTrue(session.acquire())
        session.cancel()
        assertTrue(entered.await(1, TimeUnit.SECONDS))
        val worker = Thread { session.finish(); finished.countDown() }
        worker.start()
        assertFalse(finished.await(100, TimeUnit.MILLISECONDS))
        release.countDown()
        assertTrue(finished.await(1, TimeUnit.SECONDS))
        worker.join(1000)
        assertEquals(1, calls.get())
    }

    @Test fun cancelledWaitingSessionNeverCleansUpPredecessor() {
        val calls = AtomicInteger()
        val session = SessionCleanup { calls.incrementAndGet() }
        session.cancel()
        assertFalse(session.acquire())
        session.finish()
        assertEquals(0, calls.get())
    }

    @Test fun successorWaitsUntilCancelledOwnerCleanupReturns() {
        val handoff = SessionHandoff()
        val old = handoff.next()
        val next = handoff.next()
        val cleanupEntered = CountDownLatch(1)
        val releaseCleanup = CountDownLatch(1)
        val successorEntered = CountDownLatch(1)
        val cleanup = SessionCleanup {
            cleanupEntered.countDown()
            releaseCleanup.await()
        }
        assertTrue(old.awaitPredecessor())
        assertTrue(cleanup.acquire())
        cleanup.cancel()
        assertTrue(cleanupEntered.await(1, TimeUnit.SECONDS))
        val oldWorker = Thread { cleanup.finish(); old.finish() }
        val newWorker = Thread { if (next.awaitPredecessor()) successorEntered.countDown() }
        oldWorker.start()
        newWorker.start()
        assertFalse(successorEntered.await(100, TimeUnit.MILLISECONDS))
        releaseCleanup.countDown()
        assertTrue(successorEntered.await(1, TimeUnit.SECONDS))
        oldWorker.join(1000)
        newWorker.join(1000)
        assertFalse(oldWorker.isAlive)
        assertFalse(newWorker.isAlive)
    }

    @Test fun cleanupFailurePreventsSuccessfulFinish() {
        val session = SessionCleanup { throw IllegalStateException("cleanup failed") }
        assertTrue(session.acquire())
        try {
            session.finish()
            fail("cleanup failure was lost")
        } catch (error: IllegalStateException) {
            assertEquals("cleanup failed", error.message)
        }
    }

    @Test fun naturalFinishCleansExactlyOnce() {
        val calls = AtomicInteger()
        val session = SessionCleanup { calls.incrementAndGet() }
        assertTrue(session.acquire())
        session.finish()
        session.cancel()
        session.finish()
        assertEquals(1, calls.get())
    }
}
