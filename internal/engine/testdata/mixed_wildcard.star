rule_id = "test-mixed"
event_types = ["*", "content"]
priority = 50

def evaluate(event):
    return verdict("approve")
