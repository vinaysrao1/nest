rule_id = "cat-content"
event_types = ["post"]
priority = 90

def evaluate(event):
    text = event["payload"].get("text", "")
    lower_text = ""
    for ch in text.elems():
        if ch >= "A" and ch <= "Z":
            lower_text += chr(ord(ch) + 32)
        else:
            lower_text += ch

    if "cat" in lower_text:
        return verdict("review", reason="contains 'cat'",
                       actions=["webhook-notify", "mrt-review"])

    return verdict("approve")
