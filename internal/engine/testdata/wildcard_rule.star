rule_id = "test-wildcard"
event_types = ["*"]
priority = 10

def evaluate(event):
    return verdict("approve")
