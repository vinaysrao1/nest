rule_id = "approve-rule"
event_types = ["content"]
priority = 10

def evaluate(event):
    return verdict("approve")
