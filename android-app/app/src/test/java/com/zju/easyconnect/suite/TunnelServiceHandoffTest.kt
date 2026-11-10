package com.zju.easyconnect.suite

import android.content.Intent
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [33])
class TunnelServiceHandoffTest {
    private fun awaitWithoutInterruption(latch: CountDownLatch) {
        while (true) {
            try {
                latch.await()
                return
            } catch (_: InterruptedException) {
                // The fake holds the exact interleaving even when the service interrupts its worker.
            }
        }
    }

    @Test fun cancelledOldLoginCannotStartSuccessorBeforeClientCleanup() {
        val firstLogin = CountDownLatch(1)
        val releaseFirst = CountDownLatch(1)
        val cleanupStarted = CountDownLatch(1)
        val releaseCleanup = CountDownLatch(1)
        val secondLogin = CountDownLatch(1)
        val releaseSecond = CountDownLatch(1)
        val calls = AtomicInteger()
        val cleanups = AtomicInteger()
        val fake = object : MobileBridge {
            override fun loginEpoch() = 0L
            override fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long): String {
                if (calls.incrementAndGet() == 1) {
                    firstLogin.countDown()
                    awaitWithoutInterruption(releaseFirst)
                } else {
                    secondLogin.countDown()
                    awaitWithoutInterruption(releaseSecond)
                }
                return ""
            }
            override fun loginRejected() = true
            override fun dnsServer() = ""
            override fun startStack(fd: Long) = Unit
            override fun logout() {
                if (cleanups.incrementAndGet() == 1) {
                    cleanupStarted.countDown()
                    awaitWithoutInterruption(releaseCleanup)
                }
            }
            override fun stopStack() = Unit
        }
        TunnelService.mobileBridge = fake
        val controller = Robolectric.buildService(TunnelService::class.java).create()
        val service = controller.get()
        val intent = Intent(service, TunnelService::class.java).apply {
            putExtra("server", "test")
            putExtra("username", "test")
            putExtra("password", "test")
        }
        try {
            service.onStartCommand(intent, 0, 1)
            assertTrue(firstLogin.await(2, TimeUnit.SECONDS))
            service.onStartCommand(null, 0, 2)
            assertTrue(cleanupStarted.await(2, TimeUnit.SECONDS))
            service.onStartCommand(intent, 0, 3)
            releaseFirst.countDown()
            assertFalse(secondLogin.await(150, TimeUnit.MILLISECONDS))
            assertTrue(TunnelService.currentState() is TunnelService.State.Connecting)
            releaseCleanup.countDown()
            assertTrue(secondLogin.await(2, TimeUnit.SECONDS))
            assertTrue(TunnelService.currentState() is TunnelService.State.Connecting)
        } finally {
            service.onStartCommand(null, 0, 4)
            releaseFirst.countDown()
            releaseCleanup.countDown()
            releaseSecond.countDown()
            controller.destroy()
            TunnelService.mobileBridge = GoMobileBridge
        }
    }
}
