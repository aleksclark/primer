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
    fun canOpen(lease: RemoteLease, nowWallMs: Long, elapsedMs: Long, boot: Int): Boolean {
        if (lease.receivedBoot != boot) return false
        if (nowWallMs >= lease.deliveryExpiresAtMs) return false
        val remaining = lease.leaseExpiresAtMs?.let { it - lease.serverNowMs } ?: return false
        if (remaining <= 0) return false
        return elapsedMs < lease.receivedElapsedMs + remaining
    }
}
