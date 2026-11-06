package activityfixture;

class ProcessOrder {
    static class Order {
        boolean valid;
        boolean reserved;
        int attempts;
        int priority;
    }

    static String processOrder(Order order) {
        if (order == null) return "missing";
        if (!order.valid || !reserve(order)) return "rejected";
        for (int i = 0; i < order.attempts; i++) {
            if (!ready(order)) continue;
            send(order);
            break;
        }
        switch (order.priority) {
            case 1:
                audit(order);
            case 2:
                notifyCustomer(order);
                break;
            default:
                archive(order);
        }
        return "processed";
    }

    static boolean reserve(Order order) { return order.reserved; }
    static boolean ready(Order order) { return true; }
    static void send(Order order) { }
    static void audit(Order order) { }
    static void notifyCustomer(Order order) { }
    static void archive(Order order) { }
}
