rule_id = "infinite-loop"
event_types = ["content"]
priority = 100

def evaluate(event):
    # range(1000000000) generates enough steps to exceed the 10M step cap.
    x = 0
    for i in range(1000000000):
        x = x + i
    return verdict("approve")
