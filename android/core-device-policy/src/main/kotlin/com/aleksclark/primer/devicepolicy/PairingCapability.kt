package com.aleksclark.primer.devicepolicy

data class PairingCapability(
    val cameraGranted: Boolean,
    val parentCanGrantCamera: Boolean,
    val permissionControllerAvailable: Boolean,
    val photoPickerAvailable: Boolean,
    val inMaintenance: Boolean,
    val message: String,
) {
    val canScan: Boolean get() = cameraGranted
    val canImportImage: Boolean get() = photoPickerAvailable && (inMaintenance || cameraGranted)
}

object PairingCapabilityPolicy {
    const val CAMERA = "android.permission.CAMERA"
    const val PERMISSION_CONTROLLER_ROLE = "android.app.role.SYSTEM_PERMISSION_CONTROLLER"

    fun evaluate(
        cameraGranted: Boolean,
        owner: Boolean,
        inMaintenance: Boolean,
        permissionControllerPackage: String?,
        photoPickerPackage: String?,
    ): PairingCapability {
        val controller = !permissionControllerPackage.isNullOrBlank()
        val picker = !photoPickerPackage.isNullOrBlank()
        val parentCanGrant = owner && inMaintenance
        val message = when {
            cameraGranted -> "Camera is granted for pairing."
            !owner -> "Camera pairing is unavailable until Student is device owner."
            parentCanGrant -> "Parent can grant camera for pairing during this maintenance window. The system permission screen is blocked in normal lock-task."
            else -> "Ask a parent to open maintenance and grant camera for pairing. The system permission screen is blocked in lock-task."
        }
        return PairingCapability(
            cameraGranted = cameraGranted,
            parentCanGrantCamera = parentCanGrant,
            permissionControllerAvailable = controller,
            photoPickerAvailable = picker,
            inMaintenance = inMaintenance,
            message = message,
        )
    }

    fun maintenanceDelegates(
        permissionControllerPackage: String?,
        photoPickerPackage: String?,
        documentsUiPackage: String?,
    ): List<String> = listOfNotNull(
        permissionControllerPackage,
        photoPickerPackage,
        documentsUiPackage,
    ).distinct()
}
