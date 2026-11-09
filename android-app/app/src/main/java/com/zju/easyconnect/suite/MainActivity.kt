package com.zju.easyconnect.suite

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.net.VpnService
import android.os.SystemClock
import android.util.Log
import android.widget.Button
import android.widget.EditText
import android.widget.TextView

class MainActivity : Activity() {

    private lateinit var statusText: TextView
    private lateinit var toggleButton: Button
    private var pendingConnection: Intent? = null
    private var authorizationStartedAt = 0L
    private val stateListener: (TunnelService.State) -> Unit = { state ->
        runOnUiThread { renderState(state) }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        val prefs = getSharedPreferences("config", MODE_PRIVATE)
        val server = findViewById<EditText>(R.id.etServer)
        val port = findViewById<EditText>(R.id.etPort)
        val username = findViewById<EditText>(R.id.etUsername)
        val password = findViewById<EditText>(R.id.etPassword)
        val totp = findViewById<EditText>(R.id.etTotp)
        statusText = findViewById(R.id.tvStatus)
        toggleButton = findViewById(R.id.btnToggle)

        server.setText(prefs.getString("server", ""))
        port.setText(prefs.getString("port", "443"))
        username.setText(prefs.getString("username", ""))
        password.setText(prefs.getString("password", ""))
        totp.setText(prefs.getString("totp", ""))

        TunnelService.onState = stateListener

        toggleButton.setOnClickListener {
            if (TunnelService.active) {
                stopService(Intent(this, TunnelService::class.java))
            } else {
                val s = server.text.toString().trim()
                val p = port.text.toString().trim().ifEmpty { "443" }
                val u = username.text.toString().trim()
                val pw = password.text.toString()
                val tp = totp.text.toString().trim()

                if (s.isEmpty() || u.isEmpty()) {
                    statusText.text = "服务器地址和账号不能为空"
                    return@setOnClickListener
                }

                prefs.edit()
                    .putString("server", s).putString("port", p)
                    .putString("username", u).putString("password", pw)
                    .putString("totp", tp)
                    .apply()

                val intent = Intent(this, TunnelService::class.java).apply {
                    putExtra("server", "$s:$p")
                    putExtra("username", u)
                    putExtra("password", pw)
                    putExtra("totp", tp)
                }
                pendingConnection = intent
                authorizationStartedAt = SystemClock.elapsedRealtime()
                Log.i("VPN", "vpn_authorization pid=${android.os.Process.myPid()} stage=prepare")
                val authorization = VpnService.prepare(this)
                if (authorization == null) {
                    startAuthorizedConnection()
                } else {
                    startActivityForResult(authorization, 1)
                }
            }
        }

        renderState(TunnelService.currentState())
    }

    override fun onDestroy() {
        if (TunnelService.onState === stateListener) TunnelService.onState = null
        super.onDestroy()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != 1) return
        if (resultCode == RESULT_OK) {
            startAuthorizedConnection()
        } else {
            pendingConnection = null
            Log.i("VPN", "vpn_authorization pid=${android.os.Process.myPid()} outcome=denied elapsed_ms=${SystemClock.elapsedRealtime() - authorizationStartedAt}")
            statusText.text = "VPN 授权被取消"
        }
    }

    private fun startAuthorizedConnection() {
        val intent = pendingConnection ?: return
        pendingConnection = null
        Log.i("VPN", "vpn_authorization pid=${android.os.Process.myPid()} outcome=granted elapsed_ms=${SystemClock.elapsedRealtime() - authorizationStartedAt}")
        startService(intent)
        renderState(TunnelService.State.Connecting)
    }

    private fun renderState(state: TunnelService.State) {
        when (state) {
            TunnelService.State.Disconnected -> {
                statusText.text = "未连接"
                toggleButton.text = "连接"
                toggleButton.isEnabled = true
            }
            TunnelService.State.Connecting -> {
                statusText.text = "登录并建立隧道…"
                toggleButton.text = "取消"
                toggleButton.isEnabled = true
            }
            TunnelService.State.Connected -> {
                statusText.text = "已连接 · 全部流量经 VPN"
                toggleButton.text = "断开"
                toggleButton.isEnabled = true
            }
            is TunnelService.State.Error -> {
                statusText.text = "出错:${state.message}"
                toggleButton.text = if (TunnelService.active) "取消" else "连接"
                toggleButton.isEnabled = true
            }
        }
    }
}
