rule_id = "openai-moderation"
event_types = ["post"]
priority = 200

# Ordered list of OpenAI moderation categories for deterministic output.
CATEGORIES = [
    "hate",
    "hate/threatening",
    "harassment",
    "harassment/threatening",
    "illicit",
    "illicit/violent",
    "self-harm",
    "self-harm/intent",
    "self-harm/instructions",
    "sexual",
    "sexual/minors",
    "violence",
    "violence/graphic",
]

def evaluate(event):
    text = event["payload"].get("text", "")
    if text == "":
        return verdict("approve")

    result = signal("openai-moderation", text)

    # Build the full score vector string.
    parts = []
    for cat in CATEGORIES:
        score = result.metadata.get(cat, 0.0)
        parts.append(cat + "=" + str(score))
    score_vector = ", ".join(parts)

    # Always enqueue to the "openai" MRT queue with the full score vector.
    enqueue("openai", reason=score_vector)

    log("openai-moderation: max_score=" + str(result.score) + " label=" + result.label)

    # If OpenAI flagged any category, route to review.
    if result.metadata.get("flagged", False):
        return verdict("review", reason="openai flagged: " + result.label + " (" + str(result.score) + ")")

    return verdict("approve")
