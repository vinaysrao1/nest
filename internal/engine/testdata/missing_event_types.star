rule_id = "test-no-events"
priority = 50

def evaluate(event):
    return verdict("approve")
