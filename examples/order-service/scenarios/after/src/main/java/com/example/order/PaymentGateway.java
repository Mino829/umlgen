package com.example.order;

public interface PaymentGateway {
    Receipt charge(Order order);
}
