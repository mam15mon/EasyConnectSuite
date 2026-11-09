package com.zju.easyconnect.suite

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.util.Log
import mobile.Mobile

class TunnelService : VpnService() {

    sealed class State {
        object Disconnected : State()
        object Connecting : State()
        object Connected : State()
        data class Error(val message: String, val retrying: Boolean = true) : State()
    }

    companion object {
        @Volatile var active: Boolean = false
            private set
        @Volatile private var state: State = State.Disconnected
        var onState: ((State) -> Unit)? = null

        fun currentState(): State = state

        private val mainHandler = Handler(Looper.getMainLooper())
        private fun publish(state: State) {
            this.state = state
            active = state != State.Disconnected && (state !is State.Error || state.retrying)
            mainHandler.post { if (this.state === state) onState?.invoke(state) }
        }
    }

    @Volatile private var wantConnected = false
    private var tunnelFd: ParcelFileDescriptor? = null
    @Volatile private var worker: Thread? = null
    @Volatile private var terminalError = false

    override fun onCreate() {
        super.onCreate()
        if (Build.VERSION.SDK_INT >= 26) {
            val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
            nm.createNotificationChannel(
                NotificationChannel("vpn", "VPN 隧道", NotificationManager.IMPORTANCE_LOW)
            )
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent == null || !intent.hasExtra("server")) {
            // 无 extras 的 start = 停止
            stopTunnel()
            return START_NOT_STICKY
        }

        if (wantConnected) return START_NOT_STICKY
        wantConnected = true
        terminalError = false
        publish(State.Connecting)

        val server = intent.getStringExtra("server")!!
        val username = intent.getStringExtra("username")!!
        val password = intent.getStringExtra("password")!!
        val totp = intent.getStringExtra("totp") ?: ""

        val thread = Thread {
            var attempt = 0
            val started = SystemClock.elapsedRealtime()
            while (wantConnected) {
                val ok = connectOnce(server, username, password, totp, attempt > 0)
                if (!wantConnected) break
                if (!ok && Mobile.loginRejected()) {
                    wantConnected = false
                    terminalError = true
                    Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=authentication outcome=rejected elapsed_ms=${SystemClock.elapsedRealtime() - started}")
                    publish(State.Error("认证被服务器拒绝", retrying = false))
                    stopSelf()
                    break
                }
                if (ok) attempt = 0
                attempt++
                val delay = retryDelay(attempt)
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=retry_wait attempt=$attempt delay_s=$delay")
                publish(State.Error("连接中断或失败,${delay}秒后重试"))
                try {
                    Thread.sleep(delay * 1000L)
                } catch (_: InterruptedException) {
                    break
                }
            }
        }
        worker = thread
        thread.start()

        return START_NOT_STICKY
    }

    /** 单次:登录 → 建立 VPN 接口 → 转发(阻塞到隧道断开)。返回是否已进入过转发。 */
    private fun connectOnce(server: String, user: String, pass: String, totp: String, isRetry: Boolean): Boolean {
        val ip = if (totp.isEmpty()) {
            Mobile.login(server, user, pass)
        } else {
            Mobile.loginWithTOTP(server, user, pass, totp)
        }
        if (!wantConnected) {
            Mobile.logout()
            return false
        }
        if (ip.isEmpty()) {
            if (!isRetry) publish(State.Error("登录失败,请检查配置"))
            return false
        }

        val dns = Mobile.dnsServer().ifEmpty { "172.31.112.1" }
        val builder = Builder()
            .setSession("EC 隧道")
            .setMtu(1400)
            .addAddress(ip, 32)
            .addRoute("0.0.0.0", 0)
            .addDnsServer(dns)
        // 本 App 自身流量不走 VPN,避免引擎到服务器的连接被回环进隧道
        try {
            builder.addDisallowedApplication(packageName)
        } catch (_: Exception) {
        }

        val pfd = builder.establish() ?: run {
            publish(State.Error("VPN 权限被拒绝"))
            wantConnected = false
            return false
        }

        startInForeground()
        tunnelFd = pfd
        publish(State.Connected)

        // startStack 阻塞直到隧道断开。不 detach:pfd 保持在 Java 侧,
        // 停止时 close() 会关闭 fd,解除 Go 侧的读阻塞
        Mobile.startStack(pfd.fd.toLong())
        tunnelFd = null

        // 隧道断开:若 fd 是我们主动关闭的(停止),循环条件已失效
        Mobile.logout()
        return true
    }

    private fun stopTunnel() {
        wantConnected = false
        worker?.interrupt()
        Mobile.logout()
        // 关闭 fd 解除 startStack 的读阻塞
        runCatching { tunnelFd?.close() }
        tunnelFd = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
        publish(State.Disconnected)
    }

    override fun onDestroy() {
        wantConnected = false
        worker?.interrupt()
        Mobile.logout()
        runCatching { tunnelFd?.close() }
        tunnelFd = null
        if (!terminalError) publish(State.Disconnected)
        super.onDestroy()
    }

    override fun onRevoke() {
        stopTunnel()
    }

    private fun startInForeground() {
        val pi = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            android.app.Notification.Builder(this, "vpn")
        } else {
            @Suppress("DEPRECATION")
            android.app.Notification.Builder(this)
        }
        val notification = builder
            .setSmallIcon(R.drawable.ic_logo)
            .setContentTitle("EC 隧道运行中")
            .setContentText("全部流量经 VPN")
            .setContentIntent(pi)
            .setOngoing(true)
            .build()
        startForeground(1, notification)
    }

    private fun retryDelay(attempt: Int): Int = minOf(3 * (1 shl minOf(attempt - 1, 7)), 300)
}
