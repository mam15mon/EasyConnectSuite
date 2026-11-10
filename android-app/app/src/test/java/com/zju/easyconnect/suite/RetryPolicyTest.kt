package com.zju.easyconnect.suite

import org.junit.Assert.assertEquals
import org.junit.Test

class RetryPolicyTest {
    @Test fun repeatedShortConnectionsIncreaseAndCapBackoff() {
        var attempt = 0
        val delays = mutableListOf<Int>()
        repeat(12) {
            attempt = nextRetryAttempt(attempt, 500L)
            delays.add(retryDelay(attempt))
        }
        assertEquals(listOf(3, 6, 12, 24, 48, 96, 192, 300, 300, 300, 300, 300), delays)
    }

    @Test fun onlyStableForwardingResetsBackoff() {
        assertEquals(5, nextRetryAttempt(4, null))
        assertEquals(5, nextRetryAttempt(4, 59_999L))
        assertEquals(1, nextRetryAttempt(4, 60_000L))
        assertEquals(3, retryDelay(nextRetryAttempt(4, 60_000L)))
    }
}
