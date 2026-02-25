rule_id = "test-syntax-error"
event_types = ["content"
priority = 50

def evaluate(event)
    return verdict("approve")
