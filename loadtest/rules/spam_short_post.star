rule_id = "spam-short-post"
event_types = ["post"]
priority = 100

def evaluate(event):
    text = event["payload"].get("text", "")
    char_count = len(text)
    entity_id = event["payload"].get("entity_id", "")

    if char_count < 10:
        count = counter(entity_id, "post", 300)
        if count > 5:
            return verdict("block", reason="spam: short post flood (" + str(count) + " posts in 5min)",
                           actions=["webhook-notify"])

    return verdict("approve")
