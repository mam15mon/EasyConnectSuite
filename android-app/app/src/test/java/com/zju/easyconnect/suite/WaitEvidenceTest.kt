package com.zju.easyconnect.suite

import java.util.Collections
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import org.junit.Assert.*
import org.junit.Test

class WaitEvidenceTest {
    @Test fun blockedCancelledWaitRecordsThreadEvidenceAndStopsAfterFinish() {
        val lines = Collections.synchronizedList(mutableListOf<String>())
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val cancelled = AtomicBoolean(false)
        val worker = Thread {
            val operation = WaitEvidence.begin(42, "wait_cleanup", { cancelled.get() }, { lines.add(it) }, 10, 30, 1)
            entered.countDown()
            release.await()
            operation.finish()
        }
        worker.start()
        assertTrue(entered.await(1, TimeUnit.SECONDS))
        cancelled.set(true)
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2)
        while (System.nanoTime() < deadline && lines.none { it.contains("phase=snapshot_end") }) Thread.sleep(10)
        release.countDown()
        worker.join(1000)
        assertFalse(worker.isAlive)
        val snapshot = synchronized(lines) { lines.toList() }
        assertTrue(snapshot.any { it.contains("phase=pending") && it.contains("cancelled=true") && it.contains("root_cause=undetermined") })
        assertTrue(snapshot.any { it.contains("thread_id=${worker.id}") && it.contains("state=WAITING") })
        assertTrue(snapshot.any { it.contains("phase=snapshot_end") && it.contains("truncated=") })
        assertEquals(1, snapshot.count { it.contains("phase=end") })
        Thread.sleep(80)
        assertEquals(snapshot.size, lines.size)
    }

    @Test fun quickCompletionDoesNotCaptureSnapshot() {
        val lines = Collections.synchronizedList(mutableListOf<String>())
        val operation = WaitEvidence.begin(43, "wait_handoff", { false }, { lines.add(it) }, 10, 100, 1)
        operation.finish()
        Thread.sleep(150)
        assertEquals(2, lines.size)
        assertTrue(lines.last().contains("phase=end outcome=success"))
    }
}
