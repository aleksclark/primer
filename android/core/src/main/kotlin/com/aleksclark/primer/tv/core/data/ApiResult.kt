package com.aleksclark.primer.tv.core.data

/**
 * Why a device API call did not produce a result. Each case maps to a distinct
 * thing the student or parent must be told, which is why this is a sealed
 * hierarchy rather than a bare exception.
 */
sealed interface ApiError {
    /** A message fit to show on screen. */
    val message: String

    /** No usable network path to the server, or it did not answer in time. */
    data class Network(override val message: String) : ApiError

    /**
     * The device token was rejected: never paired, revoked in the admin UI, or
     * the server's database was rebuilt. The app must return to pairing.
     */
    data class Unauthenticated(override val message: String) : ApiError

    /**
     * The server refused the action. For a grant request this is the watch-once
     * lockout or a closed availability window; for a heartbeat it is a grant
     * that expired before playback began.
     */
    data class Forbidden(override val message: String) : ApiError

    /** The referenced grant or item does not exist for this device. */
    data class NotFound(override val message: String) : ApiError

    /** The server is up but its media source is not configured. */
    data class Unavailable(override val message: String) : ApiError

    /** Anything else, including 5xx and unparseable bodies. */
    data class Unexpected(override val message: String, val status: Int? = null) : ApiError
}

/** A device API call's outcome. */
sealed interface ApiResult<out T> {
    data class Ok<T>(val value: T) : ApiResult<T>
    data class Err(val error: ApiError) : ApiResult<Nothing>
}

/** The value on success, or null. */
fun <T> ApiResult<T>.getOrNull(): T? = (this as? ApiResult.Ok)?.value

/** The error on failure, or null. */
fun <T> ApiResult<T>.errorOrNull(): ApiError? = (this as? ApiResult.Err)?.error

/** Maps a successful value, leaving failures untouched. */
inline fun <T, R> ApiResult<T>.map(transform: (T) -> R): ApiResult<R> = when (this) {
    is ApiResult.Ok -> ApiResult.Ok(transform(value))
    is ApiResult.Err -> this
}
