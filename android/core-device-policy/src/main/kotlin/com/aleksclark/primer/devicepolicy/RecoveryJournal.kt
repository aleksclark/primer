package com.aleksclark.primer.devicepolicy

import java.util.UUID

data class ConsumedIntent(
    val id: String,
    val kind: String,
    val ackReportId: String,
)

data class RecoverySnapshot(
    val state: RecoveryState,
    val audit: List<String> = emptyList(),
    val consumed: Map<String, ConsumedIntent> = emptyMap(),
)

data class RemoteIntentResult(
    val snapshot: RecoverySnapshot,
    val firstApply: Boolean,
    val ackReportId: String,
)

object RecoveryJournal {
    fun consume(
        snapshot: RecoverySnapshot,
        requestId: String,
        kind: String,
        nextState: RecoveryState,
        event: String,
        nowWallMs: Long,
        newAckId: () -> String = { UUID.randomUUID().toString() },
    ): RemoteIntentResult {
        require(requestId.isNotBlank())
        snapshot.consumed[requestId]?.let { existing ->
            return RemoteIntentResult(snapshot, firstApply = false, ackReportId = existing.ackReportId)
        }
        val ack = newAckId()
        val consumed = snapshot.consumed + (requestId to ConsumedIntent(requestId, kind, ack))
        val audit = snapshot.audit + "$nowWallMs:$event"
        return RemoteIntentResult(
            snapshot = snapshot.copy(state = nextState, audit = audit, consumed = consumed),
            firstApply = true,
            ackReportId = ack,
        )
    }

    fun encodeConsumed(consumed: Map<String, ConsumedIntent>): String =
        org.json.JSONArray(consumed.values.map { intent ->
            org.json.JSONObject()
                .put("id", intent.id)
                .put("kind", intent.kind)
                .put("ackReportId", intent.ackReportId)
        }).toString()

    fun decodeConsumed(raw: String?): Map<String, ConsumedIntent> {
        if (raw.isNullOrBlank()) return emptyMap()
        val array = org.json.JSONArray(raw)
        val out = linkedMapOf<String, ConsumedIntent>()
        for (i in 0 until array.length()) {
            val obj = array.getJSONObject(i)
            val id = obj.getString("id")
            out[id] = ConsumedIntent(
                id = id,
                kind = obj.optString("kind"),
                ackReportId = obj.optString("ackReportId").ifBlank { id },
            )
        }
        return out
    }

    fun encodeState(state: RecoveryState): String = org.json.JSONObject().apply {
        put("verifiers", org.json.JSONArray(state.verifiers))
        put("failures", state.failures)
        put("retryWallMs", state.retryWallMs)
        put("retryElapsedMs", state.retryElapsedMs)
        put("retryBoot", state.retryBoot)
        put("backoffMs", state.backoffMs)
        put("leaseUntilElapsedMs", state.leaseUntilElapsedMs)
        put("leaseBoot", state.leaseBoot)
    }.toString()

    fun encodeAudit(audit: List<String>): String = org.json.JSONArray(audit).toString()
}
