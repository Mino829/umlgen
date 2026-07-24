package com.example.order;

public class OrderService {
    private final OrderRepository repository;

    public OrderService(OrderRepository repository) {
        this.repository = repository;
    }

    public void place(Order order) {
        repository.save(order);
    }
}
