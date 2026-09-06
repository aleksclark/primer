package com.aleksclark.primer.student.management

data class RemoteLease(
    val intentId: String,
    val kind: String,
    val deliveryExpiresAtMs: Long,
    val leaseExpiresAtMs: Long?,
    val serverNowMs: Long,
    val receivedElapsedMs: Long,
    val receivedBoot: Int,
)

object RemoteLeasePolicy {
    fun remainingMs(lease: RemoteLease): Long {
        val remaining = lease.leaseExpiresAtMs?.let { it - lease.serverNowMs } ?: return -1
        return remaining
    }

    fun canOpen(lease: RemoteLease, requestElapsedMs: Long, responseElapsedMs: Long, requestBoot: Int, responseBoot: Int): Boolean {
        if (requestBoot < 0 || requestBoot != responseBoot || lease.receivedBoot != requestBoot) return false
        if (lease.serverNowMs >= lease.deliveryExpiresAtMs) return false
        val remaining = remainingMs(lease)
        if (remaining <= 0) return false
        if (responseElapsedMs < requestElapsedMs) return false
        return responseElapsedMs < requestElapsedMs + remaining
    }
}
