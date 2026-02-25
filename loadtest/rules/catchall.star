rule_id = "catchall-approve"
event_types = ["*"]
priority = 1

def evaluate(event):
    return verdict("approve")
