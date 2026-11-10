package com.zju.easyconnect.suite

import mobile.Mobile

/** The process-wide Go client used by a single service request. */
internal interface MobileBridge {
    fun loginEpoch(): Long
    fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long): String
    fun loginRejected(): Boolean
    fun dnsServer(): String
    fun startStack(fd: Long)
    fun logout()
    fun stopStack()
}

internal object GoMobileBridge : MobileBridge {
    override fun loginEpoch(): Long = Mobile.loginEpoch()
    override fun loginForEpoch(server: String, user: String, password: String, totp: String, epoch: Long): String =
        Mobile.loginForEpoch(server, user, password, totp, epoch)
    override fun loginRejected(): Boolean = Mobile.loginRejected()
    override fun dnsServer(): String = Mobile.dnsServer()
    override fun startStack(fd: Long) = Mobile.startStack(fd)
    override fun logout() = Mobile.logout()
    override fun stopStack() = Mobile.stopStack()
}
