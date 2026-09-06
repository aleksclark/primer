package com.aleksclark.primer.student.management

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import java.io.File
import kotlin.concurrent.thread
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.plus
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OutboxDurabilityTest {
    private fun entry(id: String, body: String, origin: String = "https://tasks.test", device: String = "device-1") =
        OutboxEntry(id, "policy-report", body, origin, device)

    @Test
    fun putIfAbsentFreezesBytesAndSurvivesRestart() {
        val dir = createTempDir(prefix = "outbox-test")
        val file = File(dir, "outbox.preferences_pb")
        try {
            val scope1 = SupervisorJob() + Dispatchers.IO
            val store1 = PreferenceDataStoreFactory.create(scope = CoroutineScope(scope1)) { file }
            val first = PreferencesManagementOutbox(store1)
            val frozen = runBlocking {
                first.putIfAbsent(entry("policy:a", """{"reportId":"r1","status":"applied"}"""))
                first.putIfAbsent(entry("policy:a", """{"reportId":"r2","status":"partial"}"""))
            }
            assertEquals("""{"reportId":"r1","status":"applied"}""", frozen.body)
            scope1.cancel()
            Thread.sleep(50)
            val scope2 = SupervisorJob() + Dispatchers.IO
            val store2 = PreferenceDataStoreFactory.create(scope = CoroutineScope(scope2)) { file }
            val restarted = PreferencesManagementOutbox(store2)
            val loaded = runBlocking { restarted.get("policy:a") }
            assertEquals("""{"reportId":"r1","status":"applied"}""", loaded?.body)
            assertEquals("https://tasks.test", loaded?.origin)
            scope2.cancel()
        } finally {
            dir.deleteRecursively()
        }
    }

    @Test
    fun concurrentPutsDoNotDropEntriesOrRewriteFrozenBody() {
        val dir = createTempDir(prefix = "outbox-conc")
        val file = File(dir, "outbox.preferences_pb")
        val scope = SupervisorJob() + Dispatchers.IO
        val store = PreferenceDataStoreFactory.create(scope = CoroutineScope(scope)) { file }
        val box = PreferencesManagementOutbox(store)
        try {
            val workers = (0 until 20).map { index ->
                thread {
                    runBlocking {
                        box.putIfAbsent(entry("id-$index", """{"n":$index}"""))
                        box.putIfAbsent(entry("shared", """{"n":$index}"""))
                    }
                }
            }
            workers.forEach { it.join() }
            val pending = runBlocking { box.pending("https://tasks.test", "device-1") }
            assertEquals(21, pending.size)
            val shared = pending.first { it.id == "shared" }
            assertTrue(shared.body.startsWith("""{"n":"""))
            val rewritten = runBlocking { box.putIfAbsent(entry("shared", """{"n":999}""")) }
            assertEquals(shared.body, rewritten.body)
            assertNotEquals("""{"n":999}""", rewritten.body)
        } finally {
            scope.cancel()
            dir.deleteRecursively()
        }
    }

    @Test
    fun foreignEnrollmentIsNotFlushed() {
        val box = InMemoryManagementOutbox()
        runBlocking {
            box.putIfAbsent(entry("old", "{}", origin = "https://old.test", device = "device-old"))
            box.putIfAbsent(entry("new", "{}", origin = "https://new.test", device = "device-new"))
            assertEquals(1, box.pending("https://new.test", "device-new").size)
            assertTrue(box.undelivered("https://old.test", "device-old"))
        }
    }
}
