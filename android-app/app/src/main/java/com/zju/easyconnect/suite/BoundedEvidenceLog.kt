package com.zju.easyconnect.suite

import java.io.File
import java.io.FileOutputStream
import java.io.IOException

/** Keeps a bounded, app-private copy of blocked-operation evidence. */
internal class BoundedEvidenceLog(directory: File, private val limit: Long = 256 * 1024L) {
    private val current = File(directory, "vpn-wait-evidence.log")
    private val previous = File(directory, "vpn-wait-evidence.log.old")

    companion object { private val lock = Any() }

    fun append(line: String) {
        val bytes = (line + "\n").toByteArray(Charsets.UTF_8)
        if (bytes.size > limit) throw IOException("evidence line exceeds file limit")
        synchronized(lock) {
            if (current.length() + bytes.size > limit) {
                if (previous.exists() && !previous.delete()) throw IOException("cannot remove old evidence")
                if (!current.renameTo(previous)) throw IOException("cannot rotate evidence")
            }
            FileOutputStream(current, true).use { it.write(bytes) }
        }
    }
}
