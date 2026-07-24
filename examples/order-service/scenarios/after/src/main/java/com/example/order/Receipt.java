package com.example.order;

import java.math.BigDecimal;

public record Receipt(String paymentId, BigDecimal amount) {
}
