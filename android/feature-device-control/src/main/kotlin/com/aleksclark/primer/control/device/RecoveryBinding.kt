package com.aleksclark.primer.control.device

import com.aleksclark.primer.security.RecoveryHpke

data class RecoveryBinding(
    val deviceId: String,
    val enrollmentPublicKey: String,
    val requestId: String,
    val keyId: String,
    val publicJson: ByteArray,
    val codes: List<String>,
    val acknowledged: Boolean = false,
) {
    fun matches(deviceId: String, enrollmentPublicKey: String?): Boolean =
        this.deviceId == deviceId && this.enrollmentPublicKey == enrollmentPublicKey

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is RecoveryBinding) return false
        return deviceId == other.deviceId &&
            enrollmentPublicKey == other.enrollmentPublicKey &&
            requestId == other.requestId &&
            keyId == other.keyId &&
            publicJson.contentEquals(other.publicJson) &&
            codes == other.codes &&
            acknowledged == other.acknowledged
    }

    override fun hashCode(): Int =
        listOf(deviceId, enrollmentPublicKey, requestId, keyId, publicJson.contentHashCode(), codes, acknowledged).hashCode()
}

object RecoveryPrep {
    fun bind(
        deviceId: String,
        enrollmentPublicKey: String,
        requestId: String,
        codes: List<String>,
        publicJson: ByteArray = RecoveryHpke.decodePublicEnrollmentKey(enrollmentPublicKey),
    ): RecoveryBinding {
        require(deviceId.isNotBlank()) { "device id is required" }
        require(enrollmentPublicKey.isNotBlank()) { "enrollment public key is required" }
        require(requestId.isNotBlank()) { "request id is required" }
        require(codes.isNotEmpty()) { "recovery codes are required" }
        val keyId = RecoveryHpke.keyId(publicJson)
        return RecoveryBinding(
            deviceId = deviceId,
            enrollmentPublicKey = enrollmentPublicKey,
            requestId = requestId,
            keyId = keyId,
            publicJson = publicJson,
            codes = codes,
        )
    }

    fun submitTarget(prepared: RecoveryBinding?, selectedDeviceId: String, acknowledged: Boolean): RecoveryBinding {
        check(prepared != null) { "Prepare recovery codes first." }
        check(prepared.deviceId == selectedDeviceId) { "Recovery codes belong to a different device. Prepare again." }
        check(acknowledged || prepared.acknowledged) { "Store recovery codes off this device before rotating." }
        return prepared
    }
}
