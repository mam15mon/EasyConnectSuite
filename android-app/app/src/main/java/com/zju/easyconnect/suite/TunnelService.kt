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
import java.util.concurrent.atomic.AtomicBoolean

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

        private val handoff = SessionHandoff()
        private val mainHandler = Handler(Looper.getMainLooper())
        private val stateLock = Any()
        private var stateOwner: ConnectionRequest? = null
        private var stateVersion = 0L

        private fun publishLocked(state: State) {
            this.state = state
            active = state != State.Disconnected && (state !is State.Error || state.retrying)
            val version = ++stateVersion
            mainHandler.post {
                val current = synchronized(stateLock) { stateVersion == version }
                if (current) onState?.invoke(state)
            }
        }

        private fun publish(request: ConnectionRequest, state: State, allowCancelled: Boolean = false) {
            synchronized(stateLock) {
                if (stateOwner === request && (allowCancelled || !request.cancelled.get())) {
                    publishLocked(state)
                }
            }
        }
    }

    private class ConnectionRequest(val handoff: SessionHandoff.Session) {
        val cancelled = AtomicBoolean(false)
        val terminal = AtomicBoolean(false)
        private var fd: ParcelFileDescriptor? = null
        val cleanup = SessionCleanup {
            val started = SystemClock.elapsedRealtime()
            val requestId = System.identityHashCode(this)
            Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=cleanup_start cancelled=${cancelled.get()}")
            closeFd()
            Mobile.logout()
            Mobile.stopStack()
            Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=cleanup_finished elapsed_ms=${SystemClock.elapsedRealtime() - started} cancelled=${cancelled.get()}")
        }

        @Synchronized fun registerFd(value: ParcelFileDescriptor): Boolean {
            if (cancelled.get()) return false
            fd = value
            return true
        }

        fun closeFd() {
            val old = synchronized(this) { fd.also { fd = null } }
            try {
                old?.close()
            } catch (error: java.io.IOException) {
                Log.e("VPN", "android_session pid=${android.os.Process.myPid()} request=${System.identityHashCode(this)} stage=tun_close outcome=failed error_type=${error.javaClass.simpleName}")
            }
        }

        fun cancel(): Boolean {
            synchronized(stateLock) {
                if (cancelled.getAndSet(true)) return false
            }
            handoff.cancel()
            cleanup.cancel()
            return true
        }
    }
    @Volatile private var currentRequest: ConnectionRequest? = null
    @Volatile private var wantConnected = false
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

        val request = synchronized(stateLock) {
            if (wantConnected) return START_NOT_STICKY
            ConnectionRequest(handoff.next()).also {
                wantConnected = true
                currentRequest = it
                terminalError = false
                stateOwner = it
                publishLocked(State.Connecting)
            }
        }
        startInForeground("正在登录并建立隧道")

        val server = intent.getStringExtra("server")!!
        val username = intent.getStringExtra("username")!!
        val password = intent.getStringExtra("password")!!
        val totp = intent.getStringExtra("totp") ?: ""

        val thread = Thread {
            val started = SystemClock.elapsedRealtime()
            val requestId = System.identityHashCode(request)
            try {
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=await_predecessor")
                if (!request.handoff.awaitPredecessor() || !request.cleanup.acquire() || request.cancelled.get()) return@Thread
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=handoff_ready elapsed_ms=${SystemClock.elapsedRealtime() - started} cancelled=false")
                var attempt = 0
                while (!request.cancelled.get()) {
                val loginEpoch = Mobile.loginEpoch()
                if (request.cancelled.get()) break
                val connectedMs = connectOnce(server, username, password, totp, attempt > 0, request, loginEpoch)
                if (request.cancelled.get() || request.terminal.get()) break
                if (connectedMs == null && Mobile.loginRejected()) {
                    Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=authentication outcome=rejected elapsed_ms=${SystemClock.elapsedRealtime() - started}")
                    finishWithError(request, "认证被服务器拒绝")
                    break
                }
                attempt = nextRetryAttempt(attempt, connectedMs)
                val delay = retryDelay(attempt)
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} stage=retry_wait attempt=$attempt delay_s=$delay connected_ms=${connectedMs ?: -1} cancelled=${!wantConnected}")
                updateNotification(request, "连接中断或失败，${delay}秒后重试")
                publish(request, State.Error("连接中断或失败,${delay}秒后重试"))
                try {
                    Thread.sleep(delay * 1000L)
                } catch (_: InterruptedException) {
                    break
                }
                }
            } catch (_: InterruptedException) {
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=await_predecessor outcome=interrupted cancelled=${request.cancelled.get()}")
            } finally {
                synchronized(stateLock) { request.cancelled.set(true) }
                request.handoff.cancel()
                try {
                    request.cleanup.finish()
                } catch (error: Throwable) {
                    Log.e("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=cleanup_finished outcome=failed elapsed_ms=${SystemClock.elapsedRealtime() - started} error_type=${error.javaClass.simpleName}")
                    throw error
                }
                request.handoff.finish()
                Log.i("VPN", "android_session pid=${android.os.Process.myPid()} request=$requestId stage=handoff_finished elapsed_ms=${SystemClock.elapsedRealtime() - started} cancelled=${request.cancelled.get()}")
            }
        }
        worker = thread
        thread.start()

        return START_NOT_STICKY
    }

    /** 单次:登录 → 建立 VPN 接口 → 转发(阻塞到隧道断开)。返回转发时长，未进入转发时返回 null。 */
    private fun connectOnce(server: String, user: String, pass: String, totp: String, isRetry: Boolean, request: ConnectionRequest, loginEpoch: Long): Long? {
        val ip = Mobile.loginForEpoch(server, user, pass, totp, loginEpoch)
        if (request.cancelled.get()) {
            return null
        }
        if (ip.isEmpty()) {
            if (!isRetry) publish(request, State.Error("登录失败,请检查配置"))
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
            Log.e("VPN", "android_session pid=${android.os.Process.myPid()} request=${System.identityHashCode(request)} stage=tun_establish outcome=denied elapsed_ms=${SystemClock.elapsedRealtime() - establishStarted} cancelled=${request.cancelled.get()}")
            finishWithError(request, "VPN 权限被拒绝")
            return null
        }

        // Keep the descriptor passed to Go open until StartStack returns. The
        // cancellation thread may close the service-owned descriptor at any time.
        val stackFd = try {
            ParcelFileDescriptor.dup(pfd.fileDescriptor)
        } catch (error: java.io.IOException) {
            pfd.close()
            throw error
        }
        var forwardingStarted = 0L
        try {
            if (!request.registerFd(pfd)) {
                pfd.close()
                return null
            }
            updateNotification(request, "IPv4 流量经 VPN")
            publish(request, State.Connected)
            forwardingStarted = SystemClock.elapsedRealtime()
            Mobile.startStack(stackFd.fd.toLong())
            return SystemClock.elapsedRealtime() - forwardingStarted
        } finally {
            try {
                stackFd.close()
            } finally {
                request.closeFd()
                if (!request.cancelled.get()) Mobile.logout()
            }
        }
    }

    private fun finishWithError(request: ConnectionRequest, message: String) {
        synchronized(stateLock) {
            if (stateOwner !== request || request.cancelled.get()) return
            request.terminal.set(true)
            wantConnected = false
            terminalError = true
            publishLocked(State.Error(message, retrying = false))
        }
        mainHandler.post {
            val shouldStop = synchronized(stateLock) {
                stateOwner === request && currentRequest === request && request.terminal.get()
            }
            if (shouldStop) stopSelf()
        }
    }

    private fun stopTunnel() {
        currentRequest?.cancel()
        wantConnected = false
        worker?.interrupt()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
        currentRequest?.let { publish(it, State.Disconnected, allowCancelled = true) }
    }

    override fun onDestroy() {
        val request = currentRequest
        if (request != null && request.cancel()) {
            wantConnected = false
            worker?.interrupt()
            stopForeground(STOP_FOREGROUND_REMOVE)
            if (!terminalError) publish(request, State.Disconnected, allowCancelled = true)
        }
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

    private fun updateNotification(request: ConnectionRequest, message: String) {
        mainHandler.post {
            val current = synchronized(stateLock) {
                stateOwner === request && currentRequest === request && !request.cancelled.get() && !request.terminal.get()
            }
            if (current) {
                val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
                nm.notify(1, notification(message))
            }
        }
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
