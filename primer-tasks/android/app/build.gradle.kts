plugins { id("com.android.application"); id("org.jetbrains.kotlin.android"); id("org.jetbrains.kotlin.plugin.compose"); id("org.jetbrains.kotlin.plugin.serialization") }

val configuredApiOrigin = providers.gradleProperty("primerApiOrigin")
 .orElse(providers.environmentVariable("PRIMER_API_ORIGIN"))
 .getOrElse("")

android { namespace="com.aleksclark.primertasks"; compileSdk=35
 defaultConfig {
  applicationId="com.aleksclark.primertasks"; minSdk=26; targetSdk=35; versionCode=1; versionName="1.0"; testInstrumentationRunner="androidx.test.runner.AndroidJUnitRunner"
  buildConfigField("String", "CONFIGURED_API_ORIGIN", configuredApiOrigin.quoteForBuildConfig())
 }
 buildFeatures { buildConfig = true }
 sourceSets["main"].java.srcDir("../../clients/kotlin/src/main/kotlin")
 compileOptions { sourceCompatibility=JavaVersion.VERSION_17; targetCompatibility=JavaVersion.VERSION_17 }
 kotlinOptions { jvmTarget="17" }
}

private fun String.quoteForBuildConfig(): String = "\"${replace("\"", "\\\\\"")}\""

val assertRequiredManifestPermissions by tasks.registering {
    doLast {
        val manifest = file("src/main/AndroidManifest.xml").readText()
        check("android.permission.CAMERA" in manifest) {
            "The Tasks APK must declare CAMERA for the primary CameraX pairing path"
        }
        check("android.permission.INTERNET" in manifest) {
            "The Tasks APK must declare INTERNET for the real pairing API path"
        }
    }
}

tasks.named("preBuild") {
    dependsOn(assertRequiredManifestPermissions)
}

dependencies {
 implementation("androidx.core:core-ktx:1.13.1"); implementation("androidx.activity:activity-compose:1.9.3")
 implementation(platform("androidx.compose:compose-bom:2024.10.01")); implementation("androidx.compose.ui:ui"); implementation("androidx.compose.ui:ui-tooling-preview"); implementation("androidx.compose.material3:material3")
 implementation("androidx.datastore:datastore-preferences:1.1.1"); implementation("androidx.camera:camera-camera2:1.4.1"); implementation("androidx.camera:camera-lifecycle:1.4.1"); implementation("androidx.camera:camera-view:1.4.1"); implementation("com.google.zxing:core:3.5.3")
 implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
 implementation("com.squareup.okhttp3:okhttp:4.12.0"); implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
 testImplementation("junit:junit:4.13.2")
 testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
 androidTestImplementation(platform("androidx.compose:compose-bom:2024.10.01"))
 androidTestImplementation("androidx.compose.ui:ui-test-junit4")
 androidTestImplementation("androidx.test.ext:junit:1.2.1")
 androidTestImplementation("androidx.test:runner:1.6.2")
 androidTestImplementation("androidx.test.uiautomator:uiautomator:2.3.0")
 debugImplementation("androidx.compose.ui:ui-test-manifest")
}
