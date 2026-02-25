rule_id = "test-block-spam"
event_types = ["content"]
priority = 100

def evaluate(event):
    text = event["payload"].get("text", "")
    if "spam" in text:
        return verdict("block", reason="contains spam", actions=["webhook-1"])
    return verdict("approve")
