package com.example.order;

public class OrderService {
    private final OrderStore store;
    private final PaymentGateway paymentGateway;

    public OrderService(OrderStore store, PaymentGateway paymentGateway) {
        this.store = store;
        this.paymentGateway = paymentGateway;
    }

    public Receipt place(Order order) {
        Receipt receipt = paymentGateway.charge(order);
        order.markPaid();
        store.save(order);
        return receipt;
    }
}
