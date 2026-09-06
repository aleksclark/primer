package com.aleksclark.primer.control.device

import com.aleksclark.primertasks.client.CredentialProvider
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class DeviceRepositoryTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun listsManagedDevicesWithParentJwt() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[]}"""))
        DeviceRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" }).list()
        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/managed-devices", request?.path)
        assertEquals("Bearer parent-jwt", request?.getHeader("Authorization"))
    }

    @Test(expected = IllegalStateException::class)
    fun rotateRecoveryRequiresOffDeviceAcknowledgement() = runBlocking {
        DeviceRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" }).rotateRecovery(
            "device-1",
            DeviceRepository.PreparedRotation(
                requestId = "req-1",
                codes = listOf("code-1"),
                publicJson = byteArrayOf(1, 2, 3),
            ),
            acknowledged = false,
        )
    }
}
