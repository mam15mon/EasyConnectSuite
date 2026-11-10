package com.zju.easyconnect.suite

import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.Assert.*
import org.junit.Test

class SessionHandoffTest {
    @Test fun successorWaitsForPredecessorCleanup() {
        val handoff = SessionHandoff()
        val old = handoff.next()
        val next = handoff.next()
        val entered = CountDownLatch(1)
        val finished = CountDownLatch(1)
        val thread = Thread {
            entered.countDown()
            if (next.awaitPredecessor()) finished.countDown()
        }
        thread.start()
        assertTrue(entered.await(1, TimeUnit.SECONDS))
        assertFalse(finished.await(100, TimeUnit.MILLISECONDS))
        old.finish()
        assertTrue(finished.await(1, TimeUnit.SECONDS))
        thread.join(1000)
        assertFalse(thread.isAlive)
    }

    @Test fun cancelledSuccessorDoesNotStartAfterCleanup() {
        val handoff = SessionHandoff()
        val old = handoff.next()
        val next = handoff.next()
        val entered = CountDownLatch(1)
        val started = CountDownLatch(1)
        val thread = Thread {
            entered.countDown()
            if (next.awaitPredecessor()) started.countDown()
        }
        thread.start()
        assertTrue(entered.await(1, TimeUnit.SECONDS))
        next.cancel()
        old.finish()
        thread.join(1000)
        assertFalse(thread.isAlive)
        assertEquals(1, started.count)
    }
    @Test fun cancelledIntermediateSessionCannotBypassOlderCleanup() {
        val handoff = SessionHandoff()
        val old = handoff.next()
        val middle = handoff.next()
        val next = handoff.next()
        middle.cancel()
        val finished = CountDownLatch(1)
        val middleThread = Thread { middle.finish() }
        val nextThread = Thread { if (next.awaitPredecessor()) finished.countDown() }
        middleThread.start()
        nextThread.start()
        assertFalse(finished.await(100, TimeUnit.MILLISECONDS))
        old.finish()
        assertTrue(finished.await(1, TimeUnit.SECONDS))
        middleThread.join(1000)
        nextThread.join(1000)
        assertFalse(middleThread.isAlive)
        assertFalse(nextThread.isAlive)
    }

}
