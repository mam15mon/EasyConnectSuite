package com.zju.easyconnect.suite

import android.content.Context
import android.content.Intent
import android.os.ParcelFileDescriptor
import android.os.Handler
import android.os.Looper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class TunHandoffInstrumentedTest {
    private fun awaitRelease(latch: CountDownLatch) {
        while (true) {
            try {
                latch.await()
                return
            } catch (_: InterruptedException) {
                // Hold the controlled interleaving after the service requests cancellation.
            }
        }
    }

    @Test fun oldDescriptorSurvivesCancellationUntilForwarderReturns() {
        val context: Context = InstrumentationRegistry.getInstrumentation().targetContext
        val oldForwarder = CountDownLatch(1)
        val newForwarder = CountDownLatch(1)
        val cleanupDone = CountDownLatch(1)
        val releaseOld = CountDownLatch(1)
        val releaseNew = CountDownLatch(1)
        val mainBlocked = CountDownLatch(1)
        val releaseMain = CountDownLatch(1)
        val calls = AtomicInteger()
        val oldFd = AtomicLong(-1)
        val firstPipe = ParcelFileDescriptor.createPipe()
        val secondPipe = ParcelFileDescriptor.createPipe()
        val tunCalls = AtomicInteger()
        TunnelService.tunFactory = { _, _, _ ->
            if (tunCalls.incrementAndGet() == 1) firstPipe[0] else secondPipe[0]
        }
        TunnelService.mobileBridge = object : MobileBridge {
            override fun loginEpoch() = 0L
            override fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long) = "10.0.0.2"
            override fun loginRejected() = false
            override fun dnsServer() = "10.0.0.1"
            override fun startStack(fd: Long) {
                if (calls.incrementAndGet() == 1) {
                    oldFd.set(fd)
                    oldForwarder.countDown()
                    awaitRelease(releaseOld)
                } else {
                    newForwarder.countDown()
                    awaitRelease(releaseNew)
                }
            }
            override fun logout() = Unit
            override fun stopStack() { cleanupDone.countDown() }
        }
        val start = Intent(context, TunnelService::class.java).apply {
            putExtra("server", "test")
            putExtra("username", "test")
            putExtra("password", "test")
        }
        val stop = Intent(context, TunnelService::class.java)
        try {
            context.startForegroundService(start)
            assertTrue("old forwarder did not start", oldForwarder.await(10, TimeUnit.SECONDS))
            Handler(Looper.getMainLooper()).post {
                mainBlocked.countDown()
                awaitRelease(releaseMain)
            }
            assertTrue("main thread did not block", mainBlocked.await(10, TimeUnit.SECONDS))
            context.startService(stop)
            context.startForegroundService(start)
            releaseMain.countDown()
            assertTrue("old cleanup did not complete", cleanupDone.await(10, TimeUnit.SECONDS))
            ParcelFileDescriptor.fromFd(oldFd.get().toInt()).use { assertTrue(it.fd >= 0) }
            assertFalse("new forwarder bypassed old cleanup", newForwarder.await(300, TimeUnit.MILLISECONDS))
            releaseOld.countDown()
            assertTrue("new forwarder did not start", newForwarder.await(10, TimeUnit.SECONDS))
            assertEquals(2, calls.get())
        } finally {
            releaseMain.countDown()
            context.startService(stop)
            releaseOld.countDown()
            releaseNew.countDown()
            TunnelService.mobileBridge = GoMobileBridge
            TunnelService.tunFactory = TunnelService.defaultTunFactory
            firstPipe[1].close()
            secondPipe[1].close()
        }
    }
}
