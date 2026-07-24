package com.example.order;

import java.util.Optional;

public interface OrderStore {
    void save(Order order);

    Optional<Order> findById(String id);
}
