package com.aleksclark.primer.tv.core.net

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/**
 * The TV server's device API, exactly as registered in
 * `server/internal/tv/api/device.go`. Paths are relative to the `/api/v1/`
 * base; the device bearer token is attached by [DeviceTokenInterceptor].
 *
 * Every method returns a raw [Response] so the interesting statuses (401, 403,
 * 404) can be mapped to domain failures instead of thrown as opaque HTTP
 * exceptions.
 */
interface TvApi {
    @POST("devices/pair")
    suspend fun pair(@Body body: PairRequestDto): Response<PairResponseDto>

    @GET("catalog")
    suspend fun catalog(): Response<CatalogResponseDto>

    @GET("now")
    suspend fun now(): Response<NowResponseDto>

    /**
     * @param day calendar day as `YYYY-MM-DD` in the channel's timezone. Omitted
     *   means the server's today, which is the only day the client can ask for
     *   without knowing the channel's zone.
     */
    @GET("schedule")
    suspend fun schedule(@Query("day") day: String? = null): Response<ScheduleResponseDto>

    /**
     * @param mode `on_demand` plays from the rotation catalog; `programmed`
     *   joins the channel's current airing at the server's offset.
     */
    @POST("media/{id}/grant")
    suspend fun grant(
        @Path("id") mediaItemId: String,
        @Query("mode") mode: String? = null,
    ): Response<GrantResponseDto>

    @POST("grants/{id}/heartbeat")
    suspend fun heartbeat(
        @Path("id") grantId: String,
        @Body body: HeartbeatRequestDto,
    ): Response<SessionResponseDto>

    @POST("grants/{id}/complete")
    suspend fun complete(
        @Path("id") grantId: String,
        @Body body: CompleteRequestDto,
    ): Response<SessionResponseDto>

    /** The APK the server publishes for sideloading. */
    @GET("app/release")
    suspend fun appRelease(): Response<AppReleaseDto>
}
