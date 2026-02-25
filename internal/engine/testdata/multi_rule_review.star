rule_id = "review-rule"
event_types = ["content"]
priority = 50

def evaluate(event):
    return verdict("review", reason="needs review")
