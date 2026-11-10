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
        startInForeground("正在登录并建立隧道")
        publish(State.Connecting)

        val server = intent.getStringExtra("server")!!
        val username = intent.getStringExtra("username")!!
        val password = intent.getStringExtra("password")!!
        val totp = intent.getStringExtra("totp") ?: ""

        val thread = Thread {
            var attempt = 0
            val started = SystemClock.elapsedRealtime()
            while (wantConnected) {
                val connectedMs = connectOnce(server, username, password, totp, attempt > 0)
                if (!wantConnected) break
                if (connectedMs == null && Mobile.loginRejected()) {
                    wantConnected = false
                    terminalError = true
                    Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=authentication outcome=rejected elapsed_ms=${SystemClock.elapsedRealtime() - started}")
                    publish(State.Error("认证被服务器拒绝", retrying = false))
                    stopSelf()
                    break
                }
                attempt = nextRetryAttempt(attempt, connectedMs)
                val delay = retryDelay(attempt)
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=retry_wait attempt=$attempt delay_s=$delay connected_ms=${connectedMs ?: -1} cancelled=${!wantConnected}")
                updateNotification("连接中断或失败，${delay}秒后重试")
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

    /** 单次:登录 → 建立 VPN 接口 → 转发(阻塞到隧道断开)。返回转发时长，未进入转发时返回 null。 */
    private fun connectOnce(server: String, user: String, pass: String, totp: String, isRetry: Boolean): Long? {
        val ip = if (totp.isEmpty()) {
            Mobile.login(server, user, pass)
        } else {
            Mobile.loginWithTOTP(server, user, pass, totp)
        }
        if (!wantConnected) {
            return null
        }
        if (ip.isEmpty()) {
            if (!isRetry) publish(State.Error("登录失败,请检查配置"))
            return null
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

        val establishStarted = SystemClock.elapsedRealtime()
        val pfd = builder.establish() ?: run {
            wantConnected = false
            terminalError = true
            Log.e("VPN", "android_session pid=${android.os.Process.myPid()} stage=tun_establish outcome=denied elapsed_ms=${SystemClock.elapsedRealtime() - establishStarted} cancelled=false")
            Mobile.logout()
            publish(State.Error("VPN 权限被拒绝", retrying = false))
            stopSelf()
            return null
        }

        updateNotification("IPv4 流量经 VPN")
        tunnelFd = pfd
        publish(State.Connected)

        // startStack 阻塞直到隧道断开。不 detach:pfd 保持在 Java 侧,
        // 停止时 close() 会关闭 fd,解除 Go 侧的读阻塞
        val forwardingStarted = SystemClock.elapsedRealtime()
        var forwardingMs = 0L
        try {
            Mobile.startStack(pfd.fd.toLong())
        } finally {
            forwardingMs = SystemClock.elapsedRealtime() - forwardingStarted
            tunnelFd = null
            val cleanupStarted = SystemClock.elapsedRealtime()
            try {
                pfd.close()
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=tun_close outcome=success elapsed_ms=${SystemClock.elapsedRealtime() - cleanupStarted} cancelled=${!wantConnected}")
            } catch (error: java.io.IOException) {
                Log.e("VPN", "android_session pid=${android.os.Process.myPid()} stage=tun_close outcome=failed elapsed_ms=${SystemClock.elapsedRealtime() - cleanupStarted} cancelled=${!wantConnected} error_type=${error.javaClass.simpleName}")
            }
            if (wantConnected) Mobile.logout()
        }
        return forwardingMs
    }

    private fun stopTunnel() {
        wantConnected = false
        worker?.interrupt()
        Mobile.logout()
        Mobile.stopStack()
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
        Mobile.stopStack()
        runCatching { tunnelFd?.close() }
        tunnelFd = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        if (!terminalError) publish(State.Disconnected)
        super.onDestroy()
    }

    override fun onRevoke() {
        stopTunnel()
    }

    private fun startInForeground(message: String) {
        val started = SystemClock.elapsedRealtime()
        startForeground(1, notification(message))
        Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=foreground_start outcome=success elapsed_ms=${SystemClock.elapsedRealtime() - started} cancelled=${!wantConnected}")
    }

    private fun updateNotification(message: String) {
        val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(1, notification(message))
    }

    private fun notification(message: String): android.app.Notification {
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
        return builder
            .setSmallIcon(R.drawable.ic_logo)
            .setContentTitle("EC 隧道运行中")
            .setContentText(message)
            .setContentIntent(pi)
            .setOngoing(true)
            .build()
    }

}
