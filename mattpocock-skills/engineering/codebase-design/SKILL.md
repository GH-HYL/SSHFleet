---
name: codebase-design
description: 设计深模块（deep module）所需的共享词汇。当用户想设计或改进某个模块的接口、寻找深化机会、决定接缝放在哪里、让代码更可测试或更便于 AI 导航，或者当另一个技能需要深模块词汇时使用。
---

# Codebase Design（代码库设计）

设计**深模块（deep module）**：小接口背后藏着大量行为，放在干净的接缝（seam）处，并且可以通过那个接口来测试。凡是代码正在被设计或重构的地方，都使用这套语言和这些原则。目标是给调用方带来杠杆（leverage）、给维护者带来局部性（locality）、给所有人带来可测试性。

## 词表（glossary）

严格使用这些术语：不要替换成 "component"、"service"、"API" 或 "boundary"。语言一致就是全部意义所在。

**模块（Module）**：任何有接口和实现的东西。刻意与规模无关：一个函数、类、包，或者横跨多个层次的切片。_避免_：unit、component、service。

**接口（Interface）**：调用方为了正确使用该模块所必须知道的一切：类型签名，但也包括不变量、顺序约束、错误模式、必需的配置，以及性能特征。_避免_：API、signature（太窄，它们只指类型层面的表面）。

**实现（Implementation）**：模块内部的东西，它的代码主体。与**适配器（Adapter）**不同：一个东西可以是实现很大的小适配器（一个 Postgres 仓储），也可以是实现很小的大适配器（一个内存假对象）。当话题是接缝时用 "adapter"，否则用 "implementation"。

**深度（Depth）**：接口处的杠杆。调用方（或测试）每学习一单位接口所能驱动的行为量。当一个模块把大量行为放在小接口背后时它是**深**的，当接口几乎和实现一样复杂时它是**浅**的。

**接缝（Seam）** _（Michael Feathers）_：一个你可以在不编辑该处的前提下改变行为的地方；也就是一个模块的接口所*坐落*的位置。接缝放在哪里是它自己的设计决策，与它背后放什么无关。_避免_：boundary（与 DDD 的限界上下文一词超载冲突）。

**适配器（Adapter）**：在接缝处满足某个接口的具体东西。描述的是*角色*（它填的是哪个槽位），而不是实质（里面是什么）。

**杠杆（Leverage）**：调用方从深度中得到的东西。每学习一单位接口就获得更多能力。一份实现能在 N 个调用点和 M 个测试上回本。

**局部性（Locality）**：维护者从深度中得到的东西。改动、bug、知识和验证集中在一处，而不是散布到各个调用方。修一次，处处已修。

## 深与浅

**深模块（Deep module）** = 小接口 + 大量实现：

```
┌─────────────────────┐
│   Small Interface   │  ← Few methods, simple params
├─────────────────────┤
│                     │
│  Deep Implementation│  ← Complex logic hidden
│                     │
└─────────────────────┘
```

**浅模块（Shallow module）** = 大接口 + 很少实现（要避免）：

```
┌─────────────────────────────────┐
│       Large Interface           │  ← Many methods, complex params
├─────────────────────────────────┤
│  Thin Implementation            │  ← Just passes through
└─────────────────────────────────┘
```

设计一个接口时，问：

- 我能减少方法的数量吗？
- 我能简化参数吗？
- 我能把更多复杂性藏在里面吗？

## 原则

- **深度是接口的属性，不是实现的属性。** 一个深模块内部可以由小的、可模拟的、可替换的部件组成；它们只是不属于接口的一部分。一个模块既可以有**内部接缝**（对其实现私有，供它自己的测试使用），也可以有位于其接口处的**外部接缝**。
- **删除测试（deletion test）。** 想象删掉这个模块。如果复杂性消失了，它就是个直通管道。如果复杂性在 N 个调用方那里重新出现，它就是物有所值的。
- **接口就是测试面（test surface）。** 调用方和测试穿过同一道接缝。如果你想*绕过*接口去测试，那这个模块多半形状不对。
- **一个适配器意味着假想的接缝。两个适配器才意味着真实的接缝。** 除非确实有东西在接缝两侧变化，否则不要引入接缝。

## 为可测试性而设计

好的接口让测试变得自然：

1. **接受依赖，不要创建依赖。**

   ```typescript
   // Testable
   function processOrder(order, paymentGateway) {}

   // Hard to test
   function processOrder(order) {
     const gateway = new StripeGateway();
   }
   ```

2. **返回结果，不要产生副作用。**

   ```typescript
   // Testable
   function calculateDiscount(cart): Discount {}

   // Hard to test
   function applyDiscount(cart): void {
     cart.total -= discount;
   }
   ```

3. **小的暴露面。** 更少的方法 = 需要的测试更少。更少的参数 = 更简单的测试搭建。

## 关系

- 一个**模块（Module）**恰好有一个**接口（Interface）**（它呈现给调用方和测试的表面）。
- **深度（Depth）**是**模块（Module）**的属性，相对于它的**接口（Interface）**来衡量。
- **接缝（Seam）**是**模块（Module）**的**接口（Interface）**所在之处。
- **适配器（Adapter）**坐落在**接缝（Seam）**处并满足**接口（Interface）**。
- **深度（Depth）**为调用方产生**杠杆（Leverage）**，为维护者产生**局部性（Locality）**。

## 已否决的说法

- **把深度当作实现行数与接口行数之比**（Ousterhout）：会奖励往实现里灌水。我们改而采用「深度即杠杆」。
- **把 "Interface" 当作 TypeScript 的 `interface` 关键字或类的公开方法**：太窄了：这里的接口包括调用方必须知道的每一个事实。
- **"Boundary"**：与 DDD 的限界上下文一词超载冲突。请说**接缝（seam）**或**接口（interface）**。

## 深入

- **给定依赖的情况下深化一簇模块**，见 [DEEPENING.md](DEEPENING.md)：依赖分类、接缝纪律，以及「替换而非分层」的测试策略。
- **探索备选接口**，见 [DESIGN-IT-TWICE.md](DESIGN-IT-TWICE.md)：拉起多个并行子 agent，把接口设计成几种截然不同的样子，再从深度、局部性和接缝位置三个方面进行比较。
