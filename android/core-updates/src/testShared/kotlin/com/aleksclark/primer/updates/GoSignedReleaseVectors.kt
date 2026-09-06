package com.aleksclark.primer.updates

import java.util.Base64

/**
 * Independent vectors. Not produced by Tink sign-then-verify of the same helper.
 *
 * - RFC 8032 §7.1 TEST 1 (empty message) against the published public key.
 * - Go `crypto/ed25519` signatures over canonical ReleaseManifest JSON using
 *   that same RFC 8032 seed. JVM unit tests are not Android 9 proof.
 */
object GoSignedReleaseVectors {
    const val RFC8032_PUBLIC_KEY_HEX = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
    const val RFC8032_EMPTY_SIGNATURE_HEX =
        "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"
    const val TRUST_ROOT_BASE64URL = "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
    const val SIGNING_KEY_ID = "ed25519-v1"
    const val SIGNER_SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    const val SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

    const val TV_PAYLOAD =
        """{"packageName":"com.aleksclark.primer.tv","channel":"stable","versionCode":2,"versionName":"Primer \"N+1\" 测试","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val TV_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5hbGVrc2NsYXJrLnByaW1lci50diIsImNoYW5uZWwiOiJzdGFibGUiLCJ2ZXJzaW9uQ29kZSI6MiwidmVyc2lvbk5hbWUiOiJQcmltZXIgXCJOKzFcIiDmtYvor5UiLCJtaW5TZGsiOjI4LCJzdXBwb3J0ZWRBYmlzIjpbImFybTY0LXY4YSJdLCJzaWduZXJTaGEyNTYiOiJhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhIiwic2hhMjU2IjoiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYiIsImJ5dGVTaXplIjoxMn0"
    const val TV_SIGNATURE_BASE64URL = "EaiOQ0WH-izCOo9UN9fHjJSnW7IRCJ6ZgHlRemOVVfUlQP9nTOseND-qEAsdu1xbaiPUA3mxrfzp4tHdYKtJDA"

    const val TV_PLAIN_PAYLOAD =
        """{"packageName":"com.aleksclark.primer.tv","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val TV_PLAIN_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5hbGVrc2NsYXJrLnByaW1lci50diIsImNoYW5uZWwiOiJzdGFibGUiLCJ2ZXJzaW9uQ29kZSI6MiwidmVyc2lvbk5hbWUiOiIwLjIuMCIsIm1pblNkayI6MjgsInN1cHBvcnRlZEFiaXMiOlsiYXJtNjQtdjhhIl0sInNpZ25lclNoYTI1NiI6ImFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWEiLCJzaGEyNTYiOiJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiIiwiYnl0ZVNpemUiOjEyfQ"
    const val TV_PLAIN_SIGNATURE_BASE64URL = "fAZx9AnYcEQZ1g-URkSIcoDqY5i3A3RmwGCbSaLmASejWEMBBgcc_EzH96cFzh7go2NYMapj7EttI6TsIyQNDQ"

    const val STUDENT_UNICODE_PAYLOAD =
        """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"Primer \"N+1\" 测试","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val STUDENT_UNICODE_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5hbGVrc2NsYXJrLnByaW1lci5zdHVkZW50IiwiY2hhbm5lbCI6InN0YWJsZSIsInZlcnNpb25Db2RlIjoyLCJ2ZXJzaW9uTmFtZSI6IlByaW1lciBcIk4rMVwiIOa1i-ivlSIsIm1pblNkayI6MjgsInN1cHBvcnRlZEFiaXMiOlsiYXJtNjQtdjhhIl0sInNpZ25lclNoYTI1NiI6ImFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWEiLCJzaGEyNTYiOiJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiIiwiYnl0ZVNpemUiOjEyfQ"
    const val STUDENT_UNICODE_SIGNATURE_BASE64URL = "pPOlYzjlRW0cqywQjTgeONqzuGx5z4znXs5NEfMGirBa0n869ixVgY8ewB_kKf5tK4DJ0L_F4ukG-r2mXnarDQ"

    const val STUDENT_PLAIN_PAYLOAD =
        """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val STUDENT_PLAIN_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5hbGVrc2NsYXJrLnByaW1lci5zdHVkZW50IiwiY2hhbm5lbCI6InN0YWJsZSIsInZlcnNpb25Db2RlIjoyLCJ2ZXJzaW9uTmFtZSI6IjAuMi4wIiwibWluU2RrIjoyOCwic3VwcG9ydGVkQWJpcyI6WyJhcm02NC12OGEiXSwic2lnbmVyU2hhMjU2IjoiYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYSIsInNoYTI1NiI6ImJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmIiLCJieXRlU2l6ZSI6MTJ9"
    const val STUDENT_PLAIN_SIGNATURE_BASE64URL = "PcM_ylwW0sRbtOrqkkN3e9UfnLu_tXOtr9qfpI2L6IqB-Jt39o4oNZJNBWvHnREpurVfcRSZ6vnHr3uVb4sjBg"

    const val STUDENT_OTHER_PAYLOAD =
        """{"packageName":"com.other","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val STUDENT_OTHER_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5vdGhlciIsImNoYW5uZWwiOiJzdGFibGUiLCJ2ZXJzaW9uQ29kZSI6MiwidmVyc2lvbk5hbWUiOiIwLjIuMCIsIm1pblNkayI6MjgsInN1cHBvcnRlZEFiaXMiOlsiYXJtNjQtdjhhIl0sInNpZ25lclNoYTI1NiI6ImFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWEiLCJzaGEyNTYiOiJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiIiwiYnl0ZVNpemUiOjEyfQ"
    const val STUDENT_OTHER_SIGNATURE_BASE64URL = "c6vm_01rElT6i0nT0VmdDOC4a97Kohk3i6rkKq7Ap5m3jaGwiJmSXqM_41ijh0ApdXbtqCa9HFHSTDUDd3wxDA"

    const val STUDENT_MINSDK_OVERFLOW_PAYLOAD =
        """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":2147483648,"supportedAbis":["arm64-v8a"],"signerSha256":"$SIGNER_SHA256","sha256":"$SHA256","byteSize":12}"""
    const val STUDENT_MINSDK_OVERFLOW_PAYLOAD_BASE64URL =
        "eyJwYWNrYWdlTmFtZSI6ImNvbS5hbGVrc2NsYXJrLnByaW1lci5zdHVkZW50IiwiY2hhbm5lbCI6InN0YWJsZSIsInZlcnNpb25Db2RlIjoyLCJ2ZXJzaW9uTmFtZSI6IjAuMi4wIiwibWluU2RrIjoyMTQ3NDgzNjQ4LCJzdXBwb3J0ZWRBYmlzIjpbImFybTY0LXY4YSJdLCJzaWduZXJTaGEyNTYiOiJhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhIiwic2hhMjU2IjoiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYiIsImJ5dGVTaXplIjoxMn0"
    const val STUDENT_MINSDK_OVERFLOW_SIGNATURE_BASE64URL = "YAPiEtG6oreYxvjkIk8hyqI_v_guNkpJOMq42knnDC5TXiRh2wdiCFMzOZRUJY-qypCyK5UvQEukNRGaJm7bDA"

    fun publicKey(): ByteArray = RFC8032_PUBLIC_KEY_HEX.chunked(2).map { it.toInt(16).toByte() }.toByteArray()

    fun emptySignatureBase64Url(): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(RFC8032_EMPTY_SIGNATURE_HEX.chunked(2).map { it.toInt(16).toByte() }.toByteArray())

    fun digestZerosForgery(publicKey: ByteArray, message: ByteArray): String {
        val digest = java.security.MessageDigest.getInstance("SHA-256").digest(publicKey + message)
        return Base64.getUrlEncoder().withoutPadding().encodeToString(digest + ByteArray(32))
    }
}
