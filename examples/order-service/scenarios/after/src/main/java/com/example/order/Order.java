package com.example.order;

import java.math.BigDecimal;

public class Order {
    private final String id;
    private final BigDecimal total;
    private OrderStatus status;

    public Order(String id, BigDecimal total) {
        this.id = id;
        this.total = total;
        this.status = OrderStatus.NEW;
    }

    public String getId() {
        return id;
    }

    public BigDecimal getTotal() {
        return total;
    }

    public OrderStatus getStatus() {
        return status;
    }

    public void markPaid() {
        status = OrderStatus.PAID;
    }
}
