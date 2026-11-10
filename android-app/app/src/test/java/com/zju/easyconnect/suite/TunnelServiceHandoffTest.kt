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

    @Test fun cancelledMiddleRequestCannotBypassOldClientCleanup() {
        val firstLogin = CountDownLatch(1)
        val releaseFirst = CountDownLatch(1)
        val cleanupStarted = CountDownLatch(1)
        val releaseCleanup = CountDownLatch(1)
        val secondLogin = CountDownLatch(1)
        val unexpectedMiddleLogin = CountDownLatch(1)
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
                    if (user == "middle") unexpectedMiddleLogin.countDown()
                    if (user == "new") secondLogin.countDown()
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
        val newerController = Robolectric.buildService(TunnelService::class.java).create()
        val newerService = newerController.get()
        var oldDestroyed = false
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
            val middle = Intent(intent).putExtra("username", "middle")
            val newest = Intent(intent).putExtra("username", "new")
            service.onStartCommand(middle, 0, 3)
            service.onStartCommand(null, 0, 4)
            newerService.onStartCommand(newest, 0, 5)
            releaseFirst.countDown()
            assertFalse(secondLogin.await(150, TimeUnit.MILLISECONDS))
            assertEquals(1, cleanups.get())
            assertTrue(TunnelService.currentState() is TunnelService.State.Connecting)
            releaseCleanup.countDown()
            assertTrue(secondLogin.await(2, TimeUnit.SECONDS))
            assertEquals(1L, unexpectedMiddleLogin.count)
            assertEquals(2, calls.get())
            controller.destroy()
            oldDestroyed = true
            assertEquals(1, cleanups.get())
            assertTrue(TunnelService.currentState() is TunnelService.State.Connecting)
        } finally {
            newerService.onStartCommand(null, 0, 6)
            releaseFirst.countDown()
            releaseCleanup.countDown()
            releaseSecond.countDown()
            if (!oldDestroyed) controller.destroy()
            newerController.destroy()
            TunnelService.mobileBridge = GoMobileBridge
        }
    }
    @Test fun workerExceptionPublishesTerminalErrorAndCleansUp() {
        val cleaned = CountDownLatch(1)
        val calls = AtomicInteger()
        TunnelService.mobileBridge = object : MobileBridge {
            override fun loginEpoch() = 0L
            override fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long): String {
                calls.incrementAndGet()
                throw IllegalStateException("private-canary")
            }
            override fun loginRejected() = false
            override fun dnsServer() = ""
            override fun startStack(fd: Long) = Unit
            override fun logout() { cleaned.countDown() }
            override fun stopStack() = Unit
        }
        val controller = Robolectric.buildService(TunnelService::class.java).create()
        val service = controller.get()
        val intent = Intent(service, TunnelService::class.java).apply {
            putExtra("server", "test")
            putExtra("username", "test")
            putExtra("password", "test")
        }
        try {
            service.onStartCommand(intent, 0, 1)
            assertTrue(cleaned.await(2, TimeUnit.SECONDS))
            val state = TunnelService.currentState()
            assertTrue(state is TunnelService.State.Error)
            state as TunnelService.State.Error
            assertFalse(state.retrying)
            assertEquals("连接失败", state.message)
            assertEquals(1, calls.get())
        } finally {
            controller.destroy()
            TunnelService.mobileBridge = GoMobileBridge
        }
    }

    @Test fun blockedCleanupPersistsRedactedEvidence() {
        val loginStarted = CountDownLatch(1)
        val releaseLogin = CountDownLatch(1)
        val cleanupStarted = CountDownLatch(1)
        val releaseCleanup = CountDownLatch(1)
        TunnelService.mobileBridge = object : MobileBridge {
            override fun loginEpoch() = 0L
            override fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long): String {
                loginStarted.countDown()
                awaitWithoutInterruption(releaseLogin)
                return ""
            }
            override fun loginRejected() = false
            override fun dnsServer() = ""
            override fun startStack(fd: Long) = Unit
            override fun logout() {
                cleanupStarted.countDown()
                awaitWithoutInterruption(releaseCleanup)
            }
            override fun stopStack() = Unit
        }
        val controller = Robolectric.buildService(TunnelService::class.java).create()
        val service = controller.get()
        val file = java.io.File(service.filesDir, "vpn-wait-evidence.log")
        file.delete()
        val intent = Intent(service, TunnelService::class.java).apply {
            putExtra("server", "test")
            putExtra("username", "test")
            putExtra("password", "PRIVATE_CANARY_FOR_TEST")
        }
        try {
            service.onStartCommand(intent, 0, 1)
            assertTrue(loginStarted.await(2, TimeUnit.SECONDS))
            service.onStartCommand(null, 0, 2)
            assertTrue(cleanupStarted.await(2, TimeUnit.SECONDS))
            releaseLogin.countDown()
            val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(50)
            var evidence = ""
            while (System.nanoTime() < deadline) {
                if (file.exists()) evidence = file.readText()
                if (evidence.contains("stage=await_cleanup") && evidence.contains("phase=pending") &&
                    (evidence.contains("captured=false") || evidence.contains("phase=snapshot_end"))) break
                Thread.sleep(100)
            }
            assertTrue(evidence.contains("stage=await_cleanup"))
            assertTrue(evidence.contains("phase=pending"))
            assertTrue(evidence.contains("cancelled=true"))
            assertTrue(evidence.contains("root_cause=undetermined"))
            assertFalse(evidence.contains("PRIVATE_CANARY_FOR_TEST"))
            assertTrue(file.length() <= 256 * 1024L)
        } finally {
            releaseLogin.countDown()
            releaseCleanup.countDown()
            controller.destroy()
            TunnelService.mobileBridge = GoMobileBridge
        }
    }

}
