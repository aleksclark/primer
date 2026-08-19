plugins { id("com.android.application"); id("org.jetbrains.kotlin.android"); id("org.jetbrains.kotlin.plugin.compose") }

android { namespace="com.aleksclark.primertasks"; compileSdk=35
 defaultConfig { applicationId="com.aleksclark.primertasks"; minSdk=26; targetSdk=35; versionCode=1; versionName="1.0"; testInstrumentationRunner="androidx.test.runner.AndroidJUnitRunner" }
 compileOptions { sourceCompatibility=JavaVersion.VERSION_17; targetCompatibility=JavaVersion.VERSION_17 }
 kotlinOptions { jvmTarget="17" }
}

dependencies {
 implementation("androidx.core:core-ktx:1.13.1"); implementation("androidx.activity:activity-compose:1.9.3")
 implementation(platform("androidx.compose:compose-bom:2024.10.01")); implementation("androidx.compose.ui:ui"); implementation("androidx.compose.ui:ui-tooling-preview"); implementation("androidx.compose.material3:material3")
 implementation("androidx.datastore:datastore-preferences:1.1.1"); implementation("androidx.camera:camera-camera2:1.4.1"); implementation("androidx.camera:camera-lifecycle:1.4.1"); implementation("androidx.camera:camera-view:1.4.1"); implementation("com.google.zxing:core:3.5.3")
 implementation("com.squareup.okhttp3:okhttp:4.12.0"); implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
 testImplementation("junit:junit:4.13.2")
}
