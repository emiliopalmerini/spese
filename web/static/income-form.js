function incomeForm() {
  return {
    categories: [],
    selectedCategory: '',
    selectedDate: '',
    loading: true,

    get selectedDay() {
      if (!this.selectedDate) return '';
      return new Date(this.selectedDate).getDate();
    },

    get selectedMonth() {
      if (!this.selectedDate) return '';
      return new Date(this.selectedDate).getMonth() + 1;
    },

    get isValid() {
      return this.selectedCategory !== '';
    },

    async init() {
      // Set today's date
      const today = new Date();
      this.selectedDate = today.toISOString().split('T')[0];

      // Load categories from page data
      try {
        const resp = await fetch('/api/income-categories');
        this.categories = await resp.json();
      } catch (e) {
        console.error('Failed to load income categories:', e);
      }
      this.loading = false;

      // Focus amount input
      this.$nextTick(() => {
        this.$refs.amountInput?.focus();
      });

      // Pre-select first category
      if (this.categories.length > 0) {
        this.selectedCategory = this.categories[0];
      }
    },

    selectCategory(category) {
      this.selectedCategory = category;
    },

    formatAmount(event) {
      let value = event.target.value;
      value = value.replace(/[^\d,\.]/g, '');
      value = value.replace(',', '.');
      event.target.value = value;
    }
  }
}
