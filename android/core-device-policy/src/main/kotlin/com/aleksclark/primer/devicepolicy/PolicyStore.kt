package com.aleksclark.primer.devicepolicy

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject

/** Only nonsensitive enforcement state is available before the user unlocks. */
data class ApprovedApp(val packageName: String, val label: String, val signers: Set<String>)

class PolicyStore(context: Context) {
    private val prefs = context.createDeviceProtectedStorageContext()
        .getSharedPreferences("student-policy", Context.MODE_PRIVATE)
    val configured: Boolean get() = prefs.getBoolean("configured", false)
    val report: String get() = prefs.getString("report", "Not applied")!!

    fun apps(): List<ApprovedApp> {
        val array = JSONArray(prefs.getString("apps", "[]")!!)
        return (0 until array.length()).map { index ->
            val item = array.getJSONObject(index)
            val certs = item.getJSONArray("signers")
            ApprovedApp(item.getString("package"), item.getString("label"),
                (0 until certs.length()).map { certs.getString(it) }.toSet())
        }
    }

    fun configure(apps: List<ApprovedApp>) {
        val json = JSONArray(apps.map {
            JSONObject().put("package", it.packageName).put("label", it.label).put("signers", JSONArray(it.signers.toList()))
        })
        check(prefs.edit().putString("apps", json.toString()).putBoolean("configured", true).commit()) {
            "Unable to persist approved apps"
        }
    }
    fun record(report: String) {
        // Avoid rewriting flash on every resume/status refresh when the readback is unchanged.
        if (report != this.report) check(prefs.edit().putString("report", report).commit()) { "Unable to persist policy report" }
    }

    fun lastRemoteRevision(): Long = prefs.getLong("remoteRevision", 0)
    fun lastRemoteOrigin(): String = prefs.getString("remoteOrigin", "").orEmpty()
    fun lastRemoteDeviceId(): String = prefs.getString("remoteDeviceId", "").orEmpty()

    fun rememberRemote(revision: Long, apps: List<ApprovedApp>, origin: String = lastRemoteOrigin(), deviceId: String = lastRemoteDeviceId()) {
        val sameEnrollment = origin == lastRemoteOrigin() && deviceId == lastRemoteDeviceId() && origin.isNotBlank()
        if (sameEnrollment) check(revision >= lastRemoteRevision()) { "Stale policy revision cannot overwrite last-known policy" }
        val json = JSONArray(apps.map {
            JSONObject().put("package", it.packageName).put("label", it.label).put("signers", JSONArray(it.signers.toList()))
        })
        check(
            prefs.edit()
                .putLong("remoteRevision", revision)
                .putString("remoteApps", json.toString())
                .putString("remoteOrigin", origin)
                .putString("remoteDeviceId", deviceId)
                .commit(),
        ) { "Unable to persist last-known remote policy" }
    }

    fun remoteApps(): List<ApprovedApp> {
        val raw = prefs.getString("remoteApps", null) ?: return emptyList()
        val array = JSONArray(raw)
        return (0 until array.length()).map { index ->
            val item = array.getJSONObject(index)
            val certs = item.getJSONArray("signers")
            ApprovedApp(item.getString("package"), item.getString("label"),
                (0 until certs.length()).map { certs.getString(it) }.toSet())
        }
    }

    fun reset() { check(prefs.edit().clear().commit()) }
}
