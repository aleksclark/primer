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

data class ConservativeLease(
    val effectiveServerNowMs: Long,
    val remainingMs: Long,
)

object RemoteLeasePolicy {
    fun conservative(
        lease: RemoteLease,
        requestElapsedMs: Long,
        applyElapsedMs: Long,
        requestBoot: Int,
        applyBoot: Int,
    ): ConservativeLease? {
        if (requestBoot < 0 || requestBoot != applyBoot || lease.receivedBoot != requestBoot) return null
        if (applyElapsedMs < requestElapsedMs) return null
        val delayMs = applyElapsedMs - requestElapsedMs
        val effectiveServerNow = lease.serverNowMs + delayMs
        if (effectiveServerNow >= lease.deliveryExpiresAtMs) return null
        val leaseExpires = lease.leaseExpiresAtMs ?: return null
        if (effectiveServerNow >= leaseExpires) return null
        val remaining = leaseExpires - effectiveServerNow
        if (remaining <= 0) return null
        return ConservativeLease(effectiveServerNow, remaining)
    }

    fun canOpen(
        lease: RemoteLease,
        requestElapsedMs: Long,
        responseElapsedMs: Long,
        requestBoot: Int,
        responseBoot: Int,
    ): Boolean = conservative(lease, requestElapsedMs, responseElapsedMs, requestBoot, responseBoot) != null

    fun remainingMs(lease: RemoteLease): Long {
        val remaining = lease.leaseExpiresAtMs?.let { it - lease.serverNowMs } ?: return -1
        return remaining
    }
}
