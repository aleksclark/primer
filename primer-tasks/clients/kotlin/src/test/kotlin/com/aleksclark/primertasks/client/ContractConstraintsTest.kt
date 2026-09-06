package com.aleksclark.primertasks.client

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ContractConstraintsTest {
    private val json = Json { ignoreUnknownKeys = false }

    @Test
    fun maxBytesCountsUtf8NotRunes() {
        val schema = obj("""{"type":"string","minLength":1,"x-maxBytes":12,"x-nonBlank":true}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"hello world!\"")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"${"é".repeat(7)}\"")))
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"${"é".repeat(6)}\"")))
    }

    @Test
    fun nonBlankRejectsWhitespace() {
        val schema = obj("""{"type":"string","minLength":1,"x-nonBlank":true}""")
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"   \"")))
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"source\"")))
    }

    @Test
    fun equalFieldsRequireIdenticalValues() {
        val schema = obj("""{"type":"object","properties":{"sequence":{"type":"integer"},"cursor":{"type":"integer"}},"x-equalFields":["sequence","cursor"]}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("""{"sequence":4,"cursor":4}""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""{"sequence":4,"cursor":5}""")))
    }

    @Test
    fun uniqueNormalizedIsCaseAndTrimInsensitive() {
        val schema = obj("""{"type":"array","items":{"type":"string","minLength":1,"x-nonBlank":true},"x-uniqueNormalized":true}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("""["Fact","Source"]""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""["Fact"," fact "]""")))
    }

    @Test
    fun unknownExtensionFailsClosedInsteadOfPassing() {
        val schema = obj("""{"type":"string","x-unknownConstraint":true}""")
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ContractConstraints.requireSupported(schema)
        }
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"ok\"")))
    }

    @Test
    fun frozenDialogueConfigSourceTextAndRubric() {
        val sourceText = obj("""{"type":"string","minLength":1,"x-maxBytes":12000,"x-nonBlank":true}""")
        assertTrue(ContractConstraints.matches(sourceText, json.parseToJsonElement("\"A bounded parent source.\"")))
        assertFalse(ContractConstraints.matches(sourceText, json.parseToJsonElement("\"${"é".repeat(6001)}\"")))
        val rubric = obj("""{"type":"array","minItems":1,"maxItems":20,"uniqueItems":true,"x-uniqueNormalized":true,"items":{"type":"string","minLength":1,"maxLength":500,"x-nonBlank":true}}""")
        assertTrue(ContractConstraints.matches(rubric, json.parseToJsonElement("""["source detail"]""")))
        assertFalse(ContractConstraints.matches(rubric, json.parseToJsonElement("""["Fact"," fact "]""")))
    }

    private fun obj(raw: String): JsonObject = json.parseToJsonElement(raw).jsonObject
}
