package com.aleksclark.primer.control.device

import com.aleksclark.primertasks.client.CredentialProvider
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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

    @Test
    fun rotateRecoveryRejectsUnboundDevice() = runBlocking {
        val prepared = RecoveryPrep.bind(
            deviceId = "device-a",
            enrollmentPublicKey = "key-a",
            requestId = "req-1",
            codes = listOf("c1"),
            publicJson = ByteArray(8) { 1 },
        )
        try {
            DeviceRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" })
                .rotateRecovery("device-b", "key-a", prepared, acknowledged = true)
            throw AssertionError("expected device mismatch")
        } catch (error: IllegalStateException) {
            assertTrue(error.message!!.contains("different device"))
        }
        assertEquals(0, server.requestCount)
    }

    @Test
    fun rotateRecoveryRequiresAcknowledgement() = runBlocking {
        val prepared = RecoveryPrep.bind(
            deviceId = "device-a",
            enrollmentPublicKey = "key-a",
            requestId = "req-1",
            codes = listOf("c1"),
            publicJson = ByteArray(8) { 1 },
        )
        try {
            DeviceRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" })
                .rotateRecovery("device-a", "key-a", prepared, acknowledged = false)
            throw AssertionError("expected acknowledgement")
        } catch (error: IllegalStateException) {
            assertTrue(error.message!!.contains("Store recovery codes"))
        }
        assertEquals(0, server.requestCount)
    }
}
