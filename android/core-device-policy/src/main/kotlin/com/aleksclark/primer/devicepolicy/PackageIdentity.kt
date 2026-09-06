package com.aleksclark.primer.devicepolicy

import android.content.pm.PackageManager
import java.security.MessageDigest

object PackageIdentity {
    @Suppress("DEPRECATION")
    fun signers(pm: PackageManager, packageName: String): Set<String> {
        val info = pm.getPackageInfo(packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo
            ?: return emptySet()
        return info.apkContentsSigners.map {
            MessageDigest.getInstance("SHA-256").digest(it.toByteArray()).joinToString("") { byte -> "%02x".format(byte) }
        }.toSet()
    }
}
