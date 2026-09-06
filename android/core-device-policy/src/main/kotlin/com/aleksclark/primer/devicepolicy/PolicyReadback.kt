package com.aleksclark.primer.devicepolicy

data class ControlReadback(
    val name: String,
    val desired: String,
    val actual: String,
    val status: String,
    val error: String? = null,
)

data class PolicyApplication(
    val revision: Long,
    val status: String,
    val controls: List<ControlReadback>,
    val summary: String,
)

object PolicyGuard {
    const val STUDENT_PACKAGE = "com.aleksclark.primer.student"

    fun sanitizeRemoteApps(
        requested: List<ApprovedApp>,
        studentPackage: String = STUDENT_PACKAGE,
        studentSigners: Set<String>,
        installedSigners: (String) -> Set<String>,
    ): Pair<List<ApprovedApp>, List<ControlReadback>> {
        val controls = mutableListOf<ControlReadback>()
        if (studentSigners.isEmpty()) {
            controls += ControlReadback(
                name = "student-package",
                desired = studentPackage,
                actual = "missing-signer",
                status = "failed",
                error = "Student signing identity is required",
            )
            return emptyList<ApprovedApp>() to controls
        }
        val kept = LinkedHashMap<String, ApprovedApp>()
        requested.forEach { app ->
            when {
                app.packageName == studentPackage && app.signers != studentSigners -> {
                    controls += ControlReadback(
                        name = "approved:${app.packageName}",
                        desired = app.signers.joinToString(),
                        actual = studentSigners.joinToString(),
                        status = "unsupported",
                        error = "Cannot replace Student signer",
                    )
                }
                app.packageName == studentPackage -> kept[app.packageName] = app
                app.signers.isEmpty() -> controls += ControlReadback(
                    name = "approved:${app.packageName}",
                    desired = "signer",
                    actual = "missing",
                    status = "unsupported",
                    error = "Approved app requires a signer",
                )
                else -> {
                    val installed = installedSigners(app.packageName)
                    if (installed.isNotEmpty() && installed != app.signers) {
                        controls += ControlReadback(
                            name = "approved:${app.packageName}",
                            desired = app.signers.joinToString(),
                            actual = installed.joinToString(),
                            status = "failed",
                            error = "Installed signer differs",
                        )
                    } else {
                        kept[app.packageName] = app
                    }
                }
            }
        }
        if (kept[studentPackage] == null) {
            kept[studentPackage] = ApprovedApp(studentPackage, "Primer Student", studentSigners)
            controls += ControlReadback(
                name = "student-home",
                desired = studentPackage,
                actual = studentPackage,
                status = "applied",
            )
        }
        return kept.values.toList() to controls
    }

    fun overallStatus(controls: List<ControlReadback>): String {
        if (controls.any { it.status == "failed" }) return "failed"
        if (controls.any { it.status == "unsupported" || it.status == "partial" }) return "partial"
        return "applied"
    }
}

data class InventoriedApp(
    val packageName: String,
    val label: String,
    val versionName: String,
    val versionCode: Long,
    val signerSha256: String,
)
