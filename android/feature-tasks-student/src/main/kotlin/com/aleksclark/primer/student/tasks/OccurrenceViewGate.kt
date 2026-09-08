package com.aleksclark.primer.student.tasks

/** UI request custody only; server mutations and credential revocation stay in
 * TasksSession. Navigation invalidates late view results, not committed work. */
internal class OccurrenceViewGate {
    private var generation = 0L

    fun invalidate() { generation += 1 }

    fun begin(token: String, origin: String, occurrenceId: String): OccurrenceViewRequest {
        invalidate()
        return OccurrenceViewRequest(token, origin, occurrenceId, generation)
    }

    fun accepts(request: OccurrenceViewRequest): Boolean = request.generation == generation
}

internal class OccurrenceViewRequest(
    val token: String,
    val origin: String,
    val occurrenceId: String,
    val generation: Long,
)
