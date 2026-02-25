rule_id = "test-no-priority"
event_types = ["content"]

def evaluate(event):
    return verdict("approve")
