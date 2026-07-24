package com.example.order;

import java.math.BigDecimal;

public class Order {
    private final String id;
    private final BigDecimal total;

    public Order(String id, BigDecimal total) {
        this.id = id;
        this.total = total;
    }

    public String getId() {
        return id;
    }

    public BigDecimal getTotal() {
        return total;
    }
}
