package com.zju.easyconnect.suite

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView

class MainActivity : Activity() {

    private lateinit var statusText: TextView
    private lateinit var toggleButton: Button

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

        TunnelService.onState = { runOnUiThread { renderState(it) } }

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
                startService(intent)
                renderState(TunnelService.State.Connecting)
            }
        }

        renderState(TunnelService.currentState())
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
                toggleButton.text = "连接"
                toggleButton.isEnabled = true
            }
        }
    }
}
