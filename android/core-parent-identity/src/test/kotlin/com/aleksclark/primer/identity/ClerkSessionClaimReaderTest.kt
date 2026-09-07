package com.aleksclark.primer.identity

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class ClerkSessionClaimReaderTest {
    @Test
    fun extractsPublicClaimsWithoutReturningJwt() {
        val jwt = jwt(
            """{"iss":"https://clerk.example","azp":"com.aleksclark.primer.control","aud":"none","sid":"sess_1","sub":"user_1"}""",
        )
        val claims = ClerkSessionClaimReader.fromJwt(jwt)!!
        assertEquals("https://clerk.example", claims.issuer)
        assertEquals("com.aleksclark.primer.control", claims.authorizedParty)
        assertEquals("none", claims.audience)
        assertTrue(claims.hasSessionId)
        assertTrue(claims.hasSubject)
        assertFalse(claims.toString().contains(jwt))
        assertFalse(claims.toString().contains("sess_1"))
        assertFalse(claims.toString().contains("user_1"))
    }

    @Test
    fun webOriginAzpIsReportedAsIs() {
        val jwt = jwt("""{"iss":"https://clerk.example","azp":"https://api.primerlms.com","sid":"s","sub":"u"}""")
        assertEquals("https://api.primerlms.com", ClerkSessionClaimReader.fromJwt(jwt)?.authorizedParty)
    }

    @Test
    fun missingAzpIsSessionAuthNotHouseholdDenial() {
        val jwt = jwt("""{"iss":"https://clerk.primerlms.com","sid":"s","sub":"u"}""")
        val claims = ClerkSessionClaimReader.fromJwt(jwt)!!
        val message = claims.sessionAuthMessage()!!
        assertTrue(message.contains("Authorized party omitted"))
        assertTrue(message.contains("not a household-membership denial"))
        assertFalse(message.contains("does not include your account"))
    }

    @Test
    fun missingOrMalformedJwtReturnsNull() {
        assertNull(ClerkSessionClaimReader.fromJwt(null))
        assertNull(ClerkSessionClaimReader.fromJwt(""))
        assertNull(ClerkSessionClaimReader.fromJwt("not-a-jwt"))
        assertNull(ClerkSessionClaimReader.fromJwt("a.b"))
    }

    private fun jwt(payload: String): String {
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(payload.toByteArray())
        return "aaa.$encoded.sig"
    }
}
