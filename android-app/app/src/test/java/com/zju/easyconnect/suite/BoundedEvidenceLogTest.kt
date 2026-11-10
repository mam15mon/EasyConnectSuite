package com.zju.easyconnect.suite

import java.io.File
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class BoundedEvidenceLogTest {
    @get:Rule val temporary = TemporaryFolder()

    @Test fun rotatesAndBoundsBothFiles() {
        val directory = temporary.newFolder()
        val log = BoundedEvidenceLog(directory, 20)
        log.append("first-line")
        log.append("second-line")
        assertEquals("first-line\n", File(directory, "vpn-wait-evidence.log.old").readText())
        assertEquals("second-line\n", File(directory, "vpn-wait-evidence.log").readText())
        repeat(5) { log.append("line-$it") }
        assertTrue(File(directory, "vpn-wait-evidence.log").length() <= 20)
        assertTrue(File(directory, "vpn-wait-evidence.log.old").length() <= 20)
    }

    @Test fun oversizedEntryIsRejectedWithoutWriting() {
        val directory = temporary.newFolder()
        val log = BoundedEvidenceLog(directory, 5)
        try {
            log.append("12345")
            fail("oversized entry was accepted")
        } catch (_: java.io.IOException) {
            assertFalse(File(directory, "vpn-wait-evidence.log").exists())
        }
    }
}
