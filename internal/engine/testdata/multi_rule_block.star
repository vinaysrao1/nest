rule_id = "block-rule"
event_types = ["content"]
priority = 100

def evaluate(event):
    return verdict("block", reason="blocked")
