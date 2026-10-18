# GLOSSARY.md 格式

## 结构

```md
# {Context Name}

{One or two sentence description of what this context is and why it exists.}

## Language

**Order**:
{A one or two sentence description of the term}
_Avoid_: Purchase, transaction

**Invoice**:
A request for payment sent to a customer after delivery.
_Avoid_: Bill, payment request

**Customer**:
A person or organization that places orders.
_Avoid_: Client, buyer, account
```

## 规则

- **要有主见。**当同一个概念存在多个说法时，挑出最好的那个，把其余的列在 `_Avoid_` 下。
- **定义要紧凑。**最多一两句话。定义它*是*什么，而不是它*做*什么。
- **只收录本项目上下文特有的术语。**通用编程概念（超时、错误类型、工具类模式）不属于这里，即使项目大量使用它们。添加一个术语之前先问：这是本上下文独有的概念，还是通用编程概念？只有前者才属于这里。
- **自然形成簇时，把术语归到子标题下。**如果所有术语都属于同一个内聚领域，平铺一张清单就好。

## 单上下文与多上下文仓库

**单上下文（多数仓库）：**仓库根目录一个 `GLOSSARY.md`。

**多上下文：**仓库根目录的 `GLOSSARY-MAP.md` 列出各个上下文、它们的位置，以及它们之间如何关联：

```md
# Glossary Map

## Contexts

- [Ordering](./src/ordering/GLOSSARY.md): receives and tracks customer orders
- [Billing](./src/billing/GLOSSARY.md): generates invoices and processes payments
- [Fulfillment](./src/fulfillment/GLOSSARY.md): manages warehouse picking and shipping

## Relationships

- **Ordering → Fulfillment**: Ordering emits `OrderPlaced` events; Fulfillment consumes them to start picking
- **Fulfillment → Billing**: Fulfillment emits `ShipmentDispatched` events; Billing consumes them to generate invoices
- **Ordering ↔ Billing**: Shared types for `CustomerId` and `Money`
```

本技能自行推断适用哪种结构：

- 如果存在 `GLOSSARY-MAP.md`，读它来找出各个上下文
- 如果只存在根目录的 `GLOSSARY.md`，则是单上下文
- 如果两者都不存在，就在第一个术语被确定时懒创建一个根目录的 `GLOSSARY.md`

存在多个上下文时，推断当前话题关联的是哪一个。如果不清楚，就问。
